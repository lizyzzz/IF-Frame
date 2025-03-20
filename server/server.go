package server

import (
	"fmt"
	"if-frame/internal/message"
	"if-frame/internal/network"

	"net"
)

type StubMsgCallback func(m *message.Message, stub *ClientStub)

type ClientStub struct {
	msgCb      StubMsgCallback
	msgChannel *network.Channel
	closed     bool
}

type Server struct {
	address     string
	msgCb       StubMsgCallback
	listener    net.Listener
	clientStubs map[*ClientStub]bool
	stubChan    chan *ClientStub
	rmChan      chan *ClientStub
}

// Create a clientStub with net.Conn and callback function
func CreateClientStub(conn net.Conn, msgCb StubMsgCallback) *ClientStub {
	result := &ClientStub{
		closed: false,
		msgCb:  msgCb,
	}
	result.msgChannel = network.CreateChannel(conn, result.OnChannelMsg)
	return result
}

// Create a server with address and callback function
func CreateServer(addr string, MsgCb StubMsgCallback) *Server {
	return &Server{
		msgCb:       MsgCb,
		address:     addr,
		listener:    nil,
		clientStubs: make(map[*ClientStub]bool, 32),
		stubChan:    make(chan *ClientStub, 32),
		rmChan:      make(chan *ClientStub, 32),
	}
}

func (stub *ClientStub) OnChannelMsg(msg *message.Message) {
	stub.msgCb(msg, stub)
}

// send message to client,
// need to package message
func (stub *ClientStub) SendMsg(msg *message.Message) {
	stub.msgChannel.SendMsg(msg)
}

// send message to server with type,
// don't need to package message
func (stub *ClientStub) Send(msgType uint32, msgLength uint32, seq uint32, msg []byte) {
	m := message.CreateMessage(msgType, msgLength, seq, msg)
	stub.SendMsg(m)
}

// close connect
func (stub *ClientStub) Close() {
	if !stub.closed {
		stub.closed = true
		stub.msgChannel.Close()
	}
}

// get local address
func (stub *ClientStub) LocalAddr() net.Addr {
	return stub.msgChannel.Conn.LocalAddr()
}

// get remote address
func (stub *ClientStub) RemoteAddr() net.Addr {
	return stub.msgChannel.Conn.RemoteAddr()
}

func (s *Server) Start() {
	var lisErr error
	s.listener, lisErr = net.Listen("tcp", s.address)
	if lisErr != nil {
		return
	}
	fmt.Printf("server start at %s\n", s.address)
	go s.handleClientStub()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			fmt.Errorf("Accept error: %v", err)
			continue
		}
		fmt.Printf("%s connected\n", conn.RemoteAddr().String())
		stub := CreateClientStub(conn, s.msgCb)
		s.stubChan <- stub
	}
}

func (s *Server) handleClientStub() {
	for {
		select {
		case stub := <-s.stubChan:
			s.clientStubs[stub] = true
			go s.handleStubConn(stub)
		case stub := <-s.rmChan:
			delete(s.clientStubs, stub)
		}
	}
}

func (s *Server) handleStubConn(stub *ClientStub) {
	stub.msgChannel.Start()
	stub.Close()
	s.rmChan <- stub
}
