package lutron

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/cskr/pubsub"
)

type MsgType int
type Command string

const (
	Get MsgType = iota
	Set
	Watch
	Response
)

const (
	Output  Command = "OUTPUT"
	Device  Command = "DEVICE"
	Group   Command = "GROUP"
	Unknown Command = "UNKNOWN"
)

type Lutron struct {
	hostName string
	Port     string
	conn     net.Conn
	reader   *bufio.Reader
	Username string
	Password string
	// TODO make private and expect watch requests to get access to responses
	Responses chan string
	done      chan bool
	inventory Inventory
	broker    *pubsub.PubSub
}

type LutronMsg struct {
	// the lutron component number
	Id    int
	Name  string
	Value float64
	// duration in seconds for a set action
	// TODO parse > 60 seconds into string "M:SS"
	Fade float64
	// the action to take with the command, Get, Set, Watch, Default: Get
	Type MsgType
	// the integration command type - Output, Device
	Cmd Command
	// usually the button press
	Action int
	// in Unix nanos format
	Timestamp int64
	// TODO
	// Action Number - default to 1 for now
}

type ResponseWatcher struct {
	matchMsg  *LutronMsg
	incomming chan interface{}
	Responses chan *LutronMsg
	stop      chan bool
}

// custom io scanner splitter
// splits on either '>' or '\n' as depending on whether
// the session is at a prompt - or just sent a change event
func lutronSplitter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	delim := strings.IndexAny(string(data), ">\n")
	if delim == -1 {
		// keep reading
		return 0, nil, nil
	}
	// else split the token
	return delim + 1, data[:delim], nil
}

func NewLutron(hostName, inventoryPath string) *Lutron {
	inv := NewCasetaInventory(inventoryPath)

	l := &Lutron{
		hostName:  hostName,
		Port:      "23",
		Username:  "lutron",
		Password:  "integration",
		Responses: make(chan string, 5),
		done:      make(chan bool),
		inventory: inv,
	}
	l.broker = pubsub.New(10)
	return l
}

// TODO make private when watch done
// pulls from channel until it finds response
func (l *Lutron) GetResponse() (r string, err error) {
	for {
		r = <-l.Responses
		// ignore zero length blank responses
		if len(r) > 0 {
			// TODO turn to logging
			// fmt.Println("popped ", r)
			// ignore GNET prompts
			if string(r[:1]) == "~" {
				return r, nil
			}
			// else continue
		}
	}
}

func (l *Lutron) Connect() error {

	conn, err := net.Dial("tcp", l.hostName+":"+l.Port)
	if err != nil {
		return err
	}
	l.conn = conn
	loginReader := bufio.NewReader(l.conn)
	l.reader = loginReader
	// TODO turn to logging
	fmt.Printf("Connection established between %s and localhost.\n", l.hostName)
	fmt.Printf("Local Address : %s \n", l.conn.LocalAddr().String())
	fmt.Printf("Remote Address : %s \n", l.conn.RemoteAddr().String())
	message, _ := loginReader.ReadString(':')
	fmt.Print("Message from server: " + message + "\n")
	// send to socket
	fmt.Fprintf(conn, l.Username+"\n")
	// listen for reply
	message, _ = loginReader.ReadString(':')
	fmt.Print("Message from server: " + message + "\n")
	fmt.Fprintf(l.conn, l.Password+"\n")
	message, _ = loginReader.ReadString('>')
	fmt.Print("prompt ready: " + message + "\n")
	// TODO set up scanner on l.conn
	scanner := bufio.NewScanner(l.conn)
	scanner.Split(lutronSplitter)
	go func() {
		re := regexp.MustCompile(
			// ^~(?P<command>[^,]+),(?P<id>\d+),(?P<action>\d+)(?:,(?P<value>\d+(?:\.\d+)?))?$
			`^~(?P<command>[^,]+),` + // the the commmand
				`(?P<id>\d+),` +
				`(?P<action>\d+)` +
				`(?:,(?P<value>\d+` + //values are optional
				`(?:\.\d+)?` + // not all values are floats
				`))?$`) // end optional value capture
		for scanner.Scan() {
			scannedMsg := strings.TrimSpace(scanner.Text())
			// fmt.Printf("scannedMsg: %v\n", scannedMsg)
			select {
			case <-l.done:
				return
			// case l.Responses <- scannedMsg:
			default:
				fmt.Println(scannedMsg)
			}
			response := &LutronMsg{}
			groups := re.FindStringSubmatch(scannedMsg)
			if len(groups) == 0 {
				// fmt.Println("no groups")
				continue
			}
			lutronItems := make(map[string]string)

			// fmt.Printf("%v\n", groups)
			for i, name := range re.SubexpNames() {
				if i > 0 && i <= len(groups) {
					lutronItems[name] = groups[i]
				}
			}
			// fmt.Println(lutronItems)
			switch lutronItems["command"] {
			case "OUTPUT":
				response.Cmd = Output
			case "DEVICE":
				response.Cmd = Device
			default:
				response.Cmd = Unknown
			}
			// response.Cmd = lutronItems["command"]
			// response.Cmd = "OUTPUT".(Command)
			response.Id, err = strconv.Atoi(lutronItems["id"])
			response.Action, err = strconv.Atoi(lutronItems["action"])
			if err != nil {
				log.Println(err.Error())
			}
			response.Type = Response
			response.Name, err = l.inventory.NameFromId(response.Id)
			response.Value, _ = strconv.ParseFloat(lutronItems["value"], 64)
			if err != nil {
				log.Println(err.Error())
			}
			// fmt.Printf("publishing %+v\n", response)
			l.broker.Pub(response, "responses")
		}
	}()
	return nil
}

func (l *Lutron) Disconnect() error {
	l.done <- true
	return l.conn.Close()
}

// TODO - how many API variations to support - need to have one
// with Fade
func (l *Lutron) SetById(id int, level float64) error {
	return l.Send(fmt.Sprintf("#OUTPUT,%d,1,%f", id, level))
}

func (l *Lutron) SetByName(name string, level float64) error {
	var id int
	var err error
	if id, err = l.inventory.IdFromName(name); err != nil {
		return err
	}
	return l.SetById(id, level)
}

func (l *Lutron) Send(msg string) error {
	fmt.Fprintf(l.conn, msg+"\n")
	return nil
}

func (l *Lutron) Watch(c *LutronMsg) (responses chan *LutronMsg, stop chan bool) {
	watcher := &ResponseWatcher{
		matchMsg: c,
	}
	watcher.incomming = make(chan interface{}, 5)
	watcher.Responses = make(chan *LutronMsg, 5)
	watcher.stop = make(chan bool)
	l.broker.AddSub(watcher.incomming, "responses")
	go func() {
		for {
			select {
			case msg := <-watcher.incomming:
				// match msg
				watcher.Responses <- msg.(*LutronMsg)
			case <-watcher.stop:
				l.broker.Unsub(watcher.incomming, "responses")
				close(watcher.Responses)
				return
			}
		}

	}()
	return watcher.Responses, watcher.stop
}

func (l *Lutron) SendCommand(c *LutronMsg) (resp string, err error) {
	var cmd string
	if c.Id == 0 {
		c.Id, err = l.inventory.IdFromName(c.Name)
		if err != nil {
			return "", err
		}
	}
	if c.Cmd == "" {
		c.Cmd = Output
	}

	switch c.Type {
	case Get:
		cmd = fmt.Sprintf("?%s,%d,1", c.Cmd, c.Id)
		// TODO confirm level and fade are 0
	case Set:
		cmd = fmt.Sprintf("#%s,%d,1,%.2f", c.Cmd, c.Id, c.Value)
	case Watch:
		// TODO
		// create mechanism to add a fmt.scanner on responses in a goroutine
		// with a dedicated channel for matches
		log.Fatal("Watch not implemented")
	}

	if c.Fade > 0.0 {
		cmd = fmt.Sprintf("%s,%.2f", cmd, c.Fade)
	}
	// fmt.Println("debug: ", cmd)
	l.Send(cmd)
	return l.GetResponse()
}
