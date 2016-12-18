package lutron

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
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
	Output Command = "OUTPUT"
	Device Command = "DEVICE"
	Group  Command = "GROUP"
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
}

type LutronMsg struct {
	// the lutron component number
	Id    int
	Name  string
	Level float64
	// duration in seconds for a set action
	// TODO parse > 60 seconds into string "M:SS"
	Fade float64
	// the action to take with the command, Get, Set, Watch, Default: Get
	Type MsgType
	// the integration command type - Output, Device
	Cmd Command
	// TODO
	// Action Number - default to 1 for now
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
		Responses: make(chan string),
		done:      make(chan bool),
		inventory: inv,
	}
	return l
}

// TODO make private when watch done
// pulls from channel until it finds response
// may just move to fully blocking channel fetch
func (l *Lutron) GetResponse() (r string, err error) {
	for {
		select {
		case r = <-l.Responses:
			// ignore zero length blank responses
			if len(r) > 0 {
				fmt.Println("popped ", r)
				// ignore GNET prompts
				if string(r[:1]) == "~" {
					return r, nil
				}
			}
		default:
			fmt.Println("no activity")
			time.Sleep(100 * time.Millisecond)
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
		for scanner.Scan() {
			select {
			case <-l.done:
				return
			case l.Responses <- strings.TrimSpace(scanner.Text()):
			}
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
		cmd = fmt.Sprintf("#%s,%d,1,%.2f", c.Cmd, c.Id, c.Level)
	case Watch:
		// TODO
		// create mechanism to add a fmt.scanner on responses in a goroutine
		// with a dedicated channel for matches
		log.Fatal("Watch not implemented")
	}

	if c.Fade > 0.0 {
		cmd = fmt.Sprintf("%s,%.2f", cmd, c.Fade)
	}
	fmt.Println("debug: ", cmd)
	l.Send(cmd)
	return l.GetResponse()
}
