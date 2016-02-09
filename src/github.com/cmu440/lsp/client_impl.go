// Contains the implementation of a LSP client.

package lsp

import (
	// "container/list"
	"encoding/json"
	"fmt"

	"github.com/cmu440/lspnet"
)

type client struct {
	conn         *lspnet.UDPConn
	Addr         *lspnet.UDPAddr
	Cid          int
	remove       chan int
	WritePayLoad chan []byte
	ReadFlag     chan bool
	CloseFlag    chan bool
	ReadMsg      chan Message
	// RecieveMsgList *list.List
	// SendMsgList    *list.List
	ReadStore chan Message
	RecvCid   chan int
	SeqNum    int
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {

	Client := new(client)
	Client.SeqNum = 0
	Client.remove = make(chan int)
	Client.WritePayLoad = make(chan []byte)
	Client.ReadFlag = make(chan bool)
	Client.CloseFlag = make(chan bool)
	Client.ReadMsg = make(chan Message)
	Client.ReadStore = make(chan Message, 10000)
	Client.RecvCid = make(chan int)

	// resolve UDP Address
	UDPAddr, err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		fmt.Println("Error on Resolve: ", err)
		return nil, err
	}
	Client.Addr = UDPAddr

	// Dial UDP Connection
	UDPConn, err := lspnet.DialUDP("udp", nil, UDPAddr)
	if err != nil {
		fmt.Println("Error on DialUDP: ", err)
		return nil, err
	}
	Client.conn = UDPConn

	// send connection request to server
	request := NewConnect()
	// request := NewAck(1, 1)
	MarshallMsg, err := json.Marshal(request)
	if err != nil {
		fmt.Println("Error on Marshal: ", err)
		return nil, err
	}
	//_, err = Client.conn.WriteToUDP(MarshallMsg, UDPAddr)

	_, err = Client.conn.Write(MarshallMsg)
	if err != nil {
		fmt.Println("Error on Write: ", err)
		return nil, err
	}

	// count epcho time
	// read ACK from server and unmarshall
	go Client.ReadAndUnmarshal(params)
	go Client.ClientEventHandler(params)

	// recieve ACK from server
	Client.Cid = <-Client.RecvCid

	return Client, err
}

/*====================================================*/
func (c *client) ReadAndUnmarshal(params *Params) {
	var ReadMsg [2048]byte
	var Msg *Message
	for {
		nn, err := c.conn.Read(ReadMsg[0:])
		if err != nil {
			fmt.Println("Error on ReadUDP ", err)
			return
		}
		err = json.Unmarshal(ReadMsg[0:nn], &Msg)
		if err != nil {
			fmt.Println("Error on Unmarshal ", err)
			return
		}
		c.ReadMsg <- *Msg
	}
}

func (c *client) ClientEventHandler(params *Params) {
	for {
		// fmt.Println("Client Event Handler")
		select {
		case readMsg := <-c.ReadMsg:
			// fmt.Println("Client Handler ReadMsg")
			c.HandleReadMsg(readMsg)
		case clientWrite := <-c.WritePayLoad:
			// fmt.Println("Client Handler Write")
			c.WriteMsg(clientWrite)
		case _ = <-c.CloseFlag:
			// fmt.Println("Client Handler Close")
			c.conn.Close()
		}
	}
}

/*====================================================*/
func (c *client) WriteMsg(payload []byte) error {
	c.SeqNum++
	DataMsg := NewData(c.Cid, c.SeqNum, payload, nil)
	MarshaledDataMsg, err := json.Marshal(DataMsg)
	if err != nil {
		fmt.Println("Error on Marshal ", err)
		return err
	}
	_, err = c.conn.Write(MarshaledDataMsg)
	if err != nil {
		fmt.Println("Error on Write ", err)
		return err
	}
	return nil
}

func (c *client) HandleReadMsg(readMsg Message) {

	switch readMsg.Type {
	case MsgConnect: // connection request
		// fmt.Println("Client Handle read 0")
		// server should not send connection request
		return
	case MsgData: // data
		// fmt.Println("Client Handle read 1")
		c.ReadStore <- readMsg
		Ack := NewAck(c.Cid, readMsg.SeqNum)
		c.MarshalAndSend(*Ack)
	case MsgAck: // ACK
		// fmt.Println("Client Handle read 2")

		if readMsg.SeqNum == 0 { // ACK for connection
			c.RecvCid <- readMsg.ConnID
		} else {
			// fmt.Println("MsgAck Here!")

			//
		}
	}
	return
}

func (c *client) MarshalAndSend(SendMsg Message) error {
	// marshal
	MarshalMsg, err := json.Marshal(SendMsg)
	if err != nil {
		fmt.Println("Error on Marshal: ", err)
		return err
	}
	// send marshaled msg
	_, err = c.conn.Write(MarshalMsg)
	if err != nil {
		fmt.Println("Error on Write: ", err)
		return err
	}
	return err
}

func (c *client) ConnID() int {
	return c.Cid
}

func (c *client) Read() ([]byte, error) {
	// fmt.Println("Client Read!")
	// c.ReadFlag <- true
	Msg := <-c.ReadStore
	// fmt.Println("Client Read OK!")
	return Msg.Payload, nil

}

func (c *client) Write(payload []byte) error {
	// fmt.Println("Client Write!")
	c.WritePayLoad <- payload
	return nil
}

func (c *client) Close() error {
	// fmt.Println("Client Close!")
	c.CloseFlag <- true
	return nil
}
