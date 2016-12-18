package lutron

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

type Lutron struct {
	hostName string
	Port     string
	conn     net.Conn
	// TODO use teereader to split
	// http://rodaine.com/2015/04/async-split-io-reader-in-golang/
	// then have one split just read for \n - the other for >
	// https://play.golang.org/p/P-7siRTjzA
	reader    *bufio.Reader
	Username  string
	Password  string
	Responses chan string
	done      chan bool
	// TODO device map
	inventory Inventory
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
			case l.Responses <- scanner.Text():
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
	return l.Send(fmt.Sprintf("#OUTPUT,2,%d,%f", id, level))
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
