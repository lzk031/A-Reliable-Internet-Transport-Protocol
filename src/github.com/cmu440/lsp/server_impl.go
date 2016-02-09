// Contains the implementation of a LSP server.
// this is a simple version for check point
// don't write in order and don't buffer outcome message
package lsp

import (
	"container/list"
	"encoding/json"
	// "errors"
	"fmt"
	"github.com/cmu440/lspnet"
	"strconv"
)

type virtualClient struct {
	ConnID           int
	Addr             *lspnet.UDPAddr
	ClientSeq        int // next message from this client
	ServSeq          int // next message sent to this client
	WriteMesChan     chan *Message
	UserWriteMesChan chan []byte
	CloseSig         chan bool
}

type server struct {
	Addr      *lspnet.UDPAddr
	IDCounter int
	Clients   map[*virtualClient]bool
	// AddrClientMap map[string]virtualClient // a map relate addres to client object set
	// the value to nil when client lost connection
	IdClientMap      map[int]*virtualClient // a map relate connId to client object
	IncomeMesBuf     *list.List
	MessageChan      chan *Message
	Conn             *lspnet.UDPConn
	EpochLimit       int
	EpochMillis      int
	WindowSize       int
	ReadFromConn     chan *Message
	AddClientChan    chan *lspnet.UDPAddr
	DeleteClientChan chan int
	UserReadRequest  chan bool
	UserReadMesChan  chan *Message // chan used to send message from mainhandler to read method
	CloseSig         chan bool
	CloseReadSig     chan bool
	UserStartRead    bool
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	ServerAddr, resolveErr := lspnet.ResolveUDPAddr("udp", lspnet.JoinHostPort("localhost", strconv.Itoa(port)))
	if resolveErr != nil {
		fmt.Println("falied to resolve udp address")
		return nil, resolveErr
	}
	s := server{
		IDCounter:        0,
		IdClientMap:      make(map[int]*virtualClient),
		Clients:          make(map[*virtualClient]bool),
		IncomeMesBuf:     list.New(),
		Addr:             ServerAddr,
		ReadFromConn:     make(chan *Message, 10),
		AddClientChan:    make(chan *lspnet.UDPAddr, 10),
		DeleteClientChan: make(chan int, 10),
		UserReadRequest:  make(chan bool),
		UserReadMesChan:  make(chan *Message),
		CloseSig:         make(chan bool, 1),
		CloseReadSig:     make(chan bool, 1),
		MessageChan:      make(chan *Message, 1000),
	}

	conn, err := lspnet.ListenUDP("udp", ServerAddr)
	if err != nil {
		fmt.Println("failed to listen on address")
		return nil, err
	}
	s.Conn = conn
	// fmt.Println("connection established")
	go s.MainHandler()
	go s.ReadFromClientHandler()
	return &s, nil
}

func (s *server) Read() (int, []byte, error) {
	// TODO: remove this line when you are ready to begin implementing this method.
	// select {} // Blocks indefinitely.
	// s.UserReadRequest <- true
	// mes := <-s.UserReadMesChan
	mes := <-s.MessageChan
	buf := mes.Payload
	id := mes.ConnID
	return id, buf, nil
}

func (s *server) Write(connID int, payload []byte) error {
	c := s.IdClientMap[connID]
	c.UserWriteMesChan <- payload
	return nil
	// return errors.New("not yet implemented")
}

func (s *server) CloseConn(connID int) error {
	s.DeleteClientChan <- connID
	return nil
}

func (s *server) Close() error {
	s.CloseSig <- true
	return nil
	// return errors.New("not yet implemented")
}

//#####################//
//	  helper function  //
//#####################//

func (s *server) ProcessIncomMessage(mes *Message) {
	// fmt.Println("Enter processincomemessage")
	// fmt.Println(mes)
	if mes.Type == MsgAck {
		// TODO: implement sliding window here
		// fmt.Println("Do nothing for ack now")
	} else if mes.Type == MsgData {
		// send ack back
		// fmt.Println("new msgdata: ", mes)
		ack := NewAck(mes.ConnID, mes.SeqNum)
		s.MessageChan <- mes
		c := s.IdClientMap[mes.ConnID]
		c.ClientSeq++
		c.WriteMesChan <- ack
		// if s.UserStartRead {
		// 	s.UserReadMesChan <- mes
		// 	s.UserStartRead = false
		// 	return
		// }
		// s.IncomeMesBuf.PushBack(mes)
		// PrintMesList(s.IncomeMesBuf)
	}
	// fmt.Println("exit processincomemessage")
}

func (s *server) AddClient(addr *lspnet.UDPAddr) {
	s.IDCounter++
	c := virtualClient{
		Addr:             addr,
		ConnID:           s.IDCounter,
		ClientSeq:        1,
		ServSeq:          1,
		WriteMesChan:     make(chan *Message, 10),
		UserWriteMesChan: make(chan []byte, 10),
		CloseSig:         make(chan bool, 1),
	}
	s.Clients[&c] = true
	s.IdClientMap[s.IDCounter] = &c
	go s.ClientConnectionHandler(&c)
	ack := NewAck(c.ConnID, 0)
	c.WriteMesChan <- ack
}

func (s *server) CloseAll() {
	// fmt.Println("enter close all")
	s.CloseReadSig <- true
	for c, _ := range s.Clients {
		c.CloseSig <- true
	}
	// fmt.Println("exit cloesall")
}

//#####################//
//	   event handler   //
//#####################//

func (s *server) MainHandler() {
	for {
		select {
		case <-s.CloseSig:
			s.CloseAll()
			return
		case <-s.UserReadRequest:
			// fmt.Println("start user read request")
			if s.IncomeMesBuf.Len() > 0 {
				front := s.IncomeMesBuf.Front()
				mes := front.Value.(*Message)
				s.IncomeMesBuf.Remove(front)
				s.UserReadMesChan <- mes
			} else {
				s.UserStartRead = true
			}
			// fmt.Println("end user read request")
		case mes := <-s.ReadFromConn:
			s.ProcessIncomMessage(mes)
		case addr := <-s.AddClientChan:
			// add a client
			s.AddClient(addr)

		case id := <-s.DeleteClientChan:
			// delete client
			c := s.IdClientMap[id]
			c.CloseSig <- true
			delete(s.Clients, c)
			delete(s.IdClientMap, c.ConnID)
		default:

		}
	}
}

func (s *server) ClientConnectionHandler(c *virtualClient) {
	for {
		select {
		case <-c.CloseSig:
			// fmt.Println("return from connection handler")
			return
		case mes := <-c.WriteMesChan:
			// fmt.Println("enter writemeschan")
			// fmt.Println(5)
			if mes.Type == MsgAck {
				// fmt.Println(mes)
			}
			buf, err := json.Marshal(mes)
			if err != nil {
				// fmt.Printf("connection to id:[%d] lost\n", c.ConnID)
				return
			}
			s.Conn.WriteToUDP(buf, c.Addr)
			// fmt.Println("exit writemeschan")
			// fmt.Printf("Write message '%s' to client %d\n", string(buf), c.ConnID)
		case buf := <-c.UserWriteMesChan:
			// codes about sliding window and in order concerns to be added
			// fmt.Println("start userwritemeschan")
			mes := NewData(c.ConnID, c.ServSeq, buf, nil)
			c.ServSeq++
			encodedMes, _ := json.Marshal(mes)
			s.Conn.WriteToUDP(encodedMes, c.Addr)
			// fmt.Println("end userwritemeschan")
			// fmt.Println("write successful")
		default:

		}
	}
}

func (s *server) ReadFromClientHandler() {
	for {
		select {
		case <-s.CloseReadSig:
			// fmt.Println("Return from read handler")
			return
		default:
			// fmt.Println("enter read from client handler")
			buf := make([]byte, 1024)
			var mes Message
			n, addr, err := s.Conn.ReadFromUDP(buf)
			// fmt.Println("After read")
			// fmt.Println(string(buf))
			if err != nil {
				// server.close is called all some failure happens
				// fmt.Println("Server connection lost")
				return
			}
			// fmt.Printf("message: %d, buf: %d\n", n, len(buf))
			json.Unmarshal(buf[0:n], &mes)
			if mes.Type == MsgConnect {
				s.AddClientChan <- addr
				continue
			}
			s.ReadFromConn <- &mes
			// fmt.Println("end read from client handler")
		}
	}
}
