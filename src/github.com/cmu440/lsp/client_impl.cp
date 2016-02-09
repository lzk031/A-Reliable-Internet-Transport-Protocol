// Contains the implementation of a LSP client.

package lsp

import (
	// "container/list"
	"container/list"
	"encoding/json"
	// "errors"
	"fmt"
	"github.com/cmu440/lspnet"
)

const (
	TAG = "Client"
)

type client struct {
	// TODO: implement this!
	ConnId              int
	SeqNum              int
	ServSeqNum          int // expected next message from server
	Conn                *lspnet.UDPConn
	WriteChan           chan *Message // channel used to send a message to WriteToServer routine
	ReadChan            chan *Message // channel used to send a message to ReadFromServer routine
	UserWriteMesChan    chan []byte   // channel used to send a message from write function to main handler
	UserReadMesChan     chan *Message // channel used to send a message from mainhandler to read method
	UserStartRead       bool          // indicate if there is a Read operation blocking to read message
	UserTriggerReadChan chan bool
	OutcomeMesBuf       *list.List // store message to be sent
	IncomeMesList       *list.List // store the out of order income messages
	IncomeMesBuf        *list.List // message buffered to give the user
	EpochLimit          int
	EpochMillis         int
	WindowSize          int
	IsAckArrived        []bool // keep track of a group of n consecutive messages from oldest unacked one
	WindowFront         int
	Connected           chan bool
	IsConnected         bool
	IsClosed            bool
	lock                chan bool
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separat                                                                         ed string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	// Log("Client", -1, "Start connecting to server")
	serverAddr, err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		fmt.Println("the provided hostport is not in correct format:")
		return nil, err
	}

	// Log("Client", -1, "before dial")
	conn, connErr := lspnet.DialUDP("udp", nil, serverAddr)
	// Log("Client", -1, "after dial")
	if connErr != nil {
		fmt.Println("failed to connect to the server")
		return nil, connErr
	}

	newClient := &client{
		SeqNum:              0,
		ServSeqNum:          1,
		ConnId:              -1,
		Conn:                conn,
		WriteChan:           make(chan *Message, 10),
		ReadChan:            make(chan *Message, 10),
		UserWriteMesChan:    make(chan []byte, 10),
		UserReadMesChan:     make(chan *Message),
		UserTriggerReadChan: make(chan bool, 1),
		UserStartRead:       false,
		OutcomeMesBuf:       list.New(),
		IncomeMesList:       list.New(),
		IncomeMesBuf:        list.New(),
		EpochLimit:          params.EpochLimit,
		EpochMillis:         params.EpochMillis,
		WindowSize:          params.WindowSize,
		IsAckArrived:        make([]bool, params.WindowSize),
		WindowFront:         0,
		Connected:           make(chan bool),
		IsClosed:            false,
		lock:                make(chan bool, 1),
	}

	// Log("Client", -1, "after create new client")

	go newClient.MainHandler()
	go newClient.ReadFromServerHandler()
	go newClient.WriteToServerHandler()

	// send a connect message to server and
	// wait for ack before return
	mes := NewConnect()
	newClient.SeqNum++

	newClient.WriteChan <- mes
	// use for loop to make it extensible
	// for epoch feature
	select {
	case <-newClient.Connected:
		fmt.Println("received return signal")
	}

	// Log(TAG, newClient.ConnId, "recieved connection ack")
	// Log("Client", newClient.ConnId, "connection established")
	fmt.Println("connection established")
	newClient.IsConnected = true
	return newClient, nil
}

func (c *client) ConnID() int {
	c.lock <- true
	ans := c.ConnId
	<-c.lock
	return ans
}

func (c *client) Read() ([]byte, error) {
	// Log(TAG, c.ConnId, "start to read from server")
	c.UserTriggerReadChan <- true
	// Log(TAG, c.ConnId, "waiting for message in Read")
	mes := <-c.UserReadMesChan
	return mes.Payload, nil
	// return nil, errors.New("not yet implemented")
}

// a simple Write method for check point
// some code testing connection failure to be added
func (c *client) Write(payload []byte) error {
	c.UserWriteMesChan <- payload
	return nil // to be changed
	// return errors.New("not yet implemented")
}

// a simple version, to be changed to
// allow sending of pending messages
func (c *client) Close() error {
	c.Conn.Close()
	c.IsClosed = true
	return nil
}

//#####################//
//	  helper function  //
//#####################//

// This method given the sequence number
// of the Ack message, update IsAckArrived slices
// and WindowFront
func (c *client) UpdateAck(seqNum int) {
	if seqNum == c.WindowFront {
		c.IsAckArrived[0] = true
		for c.IsAckArrived[0] == true {
			c.IsAckArrived = c.IsAckArrived[1:c.WindowSize]
			c.IsAckArrived = append(c.IsAckArrived, false)
			c.WindowFront++
		}
		// send message in outcome message buffer that should be sent
		if c.OutcomeMesBuf.Len() > 0 {
			c.FlushOutcomeBuf()
		}
	} else if seqNum > c.WindowFront {
		c.IsAckArrived[seqNum-c.WindowFront] = true
	} else {
		// Log("Client", c.ConnId, "Weired, duplicate acks coming")
	}
}

// this method will check the OutcomeMesBuff
// and send all the message that within
// current window to the server
func (c *client) FlushOutcomeBuf() {
	// fmt.Println("..........enter flush")
	// fmt.Println("current OutcomeMesBuf: ")
	// PrintMesList(c.OutcomeMesBuf)
	e := c.OutcomeMesBuf.Front()
	for e != nil {
		// fmt.Println("current node: ", e.Value)
		sequence := e.Value.(*Message).SeqNum
		// fmt.Printf("sequence: %d, wfront: %d, windowsize: %d\n", sequence, c.WindowFront, c.WindowSize)
		if sequence < c.WindowFront+c.WindowSize {
			// fmt.Printf("Write message: '%s' to server\n", string(e.Value.(*Message).Payload))
			c.WriteChan <- e.Value.(*Message)
			c.OutcomeMesBuf.Remove(e)
			e = c.OutcomeMesBuf.Front()
		} else {
			break
		}
	}
	// fmt.Println("..........exit flush")
}

func (c *client) ProcessIncomeMessage(mes *Message) {
	// fmt.Println("enter process message")
	if mes.Type == MsgAck {
		c.UpdateAck(mes.SeqNum)
		if mes.SeqNum == 0 && !c.IsConnected {
			c.lock <- true
			c.ConnId = mes.ConnID
			<-c.lock
			c.Connected <- true
		}
	} else if mes.Type == MsgData {
		if mes.SeqNum == c.ServSeqNum {
			// put this message to the top
			c.IncomeMesList.PushFront(mes)
			e := c.IncomeMesList.Front()
			// send as many ack as possible
			for e != nil {
				tempMsg := e.Value.(*Message)
				if tempMsg.SeqNum > c.ServSeqNum {
					break
				}
				// send ACK to the server, update
				// expected seqNum
				c.WriteChan <- NewAck(c.ConnId, tempMsg.SeqNum)
				c.ServSeqNum++
				prev := e
				e = e.Next()
				c.IncomeMesList.Remove(prev)
				if c.UserStartRead {
					// if there is a user read operation
					// blocking, just send the message
					// to the read method and don't stoe
					// it in IncomeMesBuf
					c.UserReadMesChan <- tempMsg
					c.UserStartRead = false
					continue
				}
				c.IncomeMesBuf.PushBack(tempMsg)
			}
		} else if mes.SeqNum > c.ServSeqNum {
			InsertIntoList(c.IncomeMesList, mes)
		} else {
			// Log("Client", c.ConnId, "Wired, same message comes again")
		}
		// fmt.Println("return from process message")
	}
}

//#####################//
//	  event handler   //
//#####################//

// this function is the main routine
// handling communication between client
// and server
func (c *client) MainHandler() {
	for {
		if c.IsClosed {
			return
		}
		// fmt.Println("before select in mainhanlder")
		select {
		case mes := <-c.ReadChan:
			c.ProcessIncomeMessage(mes)

		case buf := <-c.UserWriteMesChan:
			// user write new message to server
			// construct a new message
			// Log(TAG, c.ConnId, "************************received write signal in mainhandler")
			mes := NewData(c.ConnId, c.SeqNum, buf, Hash(buf))

			// fmt.Println("***************new message:", mes)
			c.SeqNum++
			InsertIntoList(c.OutcomeMesBuf, mes)
			// fmt.Println("insert this new message to outcomemesbuf")
			// PrintMesList(c.OutcomeMesBuf)
			c.FlushOutcomeBuf()

		case <-c.UserTriggerReadChan:
			if c.IncomeMesBuf.Len() > 0 {
				// if there is pending income message, just
				// give the first one to user
				front := c.IncomeMesBuf.Front()
				mes := c.IncomeMesBuf.Remove(front).(*Message)
				c.UserReadMesChan <- mes
			} else {
				c.UserStartRead = true
			}
		default:
			// do nothing
		}
	}
}

func (c *client) ReadFromServerHandler() {
	for {
		if c.IsClosed {
			return
		}
		buf := make([]byte, 1024)
		n, err := c.Conn.Read(buf)
		if err != nil {
			// Log("Client", c.ConnId, "Read error!")
		}
		var mes Message
		json.Unmarshal(buf[0:n], &mes)
		c.ReadChan <- &mes

	}
}

// this method will loop forever to
// try to write message to the server
func (c *client) WriteToServerHandler() {
	for {
		if c.IsClosed {
			return
		}
		select {
		case mes := <-c.WriteChan:
			if mes.Type == MsgData {
				// fmt.Printf("write a data message to server %s\n", mes.String())
				// Log(TAG, c.ConnId, "write a message to client")
			}
			buf, err := json.Marshal(mes)
			if err != nil {
				// Log("Client", c.ConnId, "error happens in marshaling")
			}
			_, writeErr := c.Conn.Write(buf)

			if writeErr != nil {
				// Log("Client", c.ConnId, "write failure")
				return
			}

		default:
			// do nothing, avoid blocking
		}
	}
}
