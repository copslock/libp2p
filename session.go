package libp2p

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/lengzhao/libp2p/crypto"
	"github.com/xtaci/smux"
	"log"
	"net"
	// "runtime/debug"
	"time"
)

const (
	messageLengthLimit = 65000
)

// PeerSession peer session.
type PeerSession struct {
	Net        *Network
	session    *smux.Session
	peerID     []byte
	isServer   bool
	remoteAddr string
	die        chan bool
}

func newSession(n *Network, conn net.Conn, server bool) *PeerSession {
	session := new(PeerSession)
	var err error
	if server {
		session.session, err = smux.Server(conn, nil)
	} else {
		session.session, err = smux.Client(conn, nil)
	}
	if err != nil {
		log.Println("fail to new smux session,", err)
		return nil
	}
	session.Net = n
	session.isServer = server
	session.die = make(chan bool)
	rAddr := conn.RemoteAddr().String()
	n.mu.Lock()
	n.peers[rAddr] = session
	n.mu.Unlock()
	session.remoteAddr = rAddr
	log.Println("new connection:", rAddr, server)
	go session.receiveMsg()
	return session
}

// Close Close
func (c *PeerSession) Close() {
	select {
	case <-c.die:
		return
	default:
		close(c.die)
		c.session.Close()
		c.Net.closeSession(c)
	}
}

func (c *PeerSession) receiveMsg() {
	defer c.Close()
	for _, plugin := range c.Net.pulgins {
		plugin.PeerConnect(c)
		defer plugin.PeerDisconnect(c)
	}
	for {
		ns, err := c.session.AcceptStream()
		if err != nil {
			log.Println("fail to AcceptStream", c.Net.address, err)
			break
		}
		go c.process(ns)
	}
}

func (c *PeerSession) process(ns *smux.Stream) {
	defer func() {
		ns.Close()
		if err := recover(); err != nil {
			fmt.Println("process painc:", err)
		}
	}()
	data := make([]byte, binary.MaxVarintLen64)
	_, err := ns.Read(data)
	if err != nil {
		log.Println("fail to read from stream:", ns.ID())
		return
	}

	l, _ := binary.Varint(data)
	if l == 0 || l > messageLengthLimit {
		log.Println("error data length:", l)
		return
	}
	data = make([]byte, l)
	buff := data
	var n int
	for {
		ln, err := ns.Read(buff)
		n += ln
		if err != nil || n >= int(l) {
			break
		}
		buff = buff[ln:]
	}
	if n < int(l) {
		log.Println("error data length:", n, "<", l)
		return
	}
	log.Println("receive data form:", c.GetRemoteAddress(), len(data))

	var msg crypto.Message
	err = proto.Unmarshal(data, &msg)
	if err != nil {
		c.Close()
		return
	}
	if bytes.Compare(msg.To, c.Net.publicKey) != 0 {
		c.Close()
		return
	}
	if c.isServer && c.peerID == nil {
		c.peerID = msg.From
	}
	if bytes.Compare(msg.From, c.peerID) != 0 {
		c.Close()
		return
	}
	if msg.Timestamp > time.Now().Add(time.Minute).UnixNano() ||
		msg.Timestamp < time.Now().Add(-time.Minute).UnixNano() {
		return
	}
	var ptr types.DynamicAny
	err = types.UnmarshalAny(msg.DataMsg, &ptr)
	if err != nil {
		c.Close()
		return
	}
	sign := msg.Sign
	msg.Sign = nil
	data, _ = proto.Marshal(&msg)
	key, ok := c.Net.keygen[msg.Keygen]
	if !ok {
		c.Close()
		return
	}
	if !key.Verify(data, sign, msg.From) {
		c.Close()
		return
	}
	log.Printf("process msg. From:%x, To:%x, Timestamp:%d\n", msg.From, msg.To, msg.Timestamp)

	newContext(c, ptr.Message)
}

// Send send message
func (c *PeerSession) Send(message proto.Message) error {
	any, err := types.MarshalAny(message)
	if err != nil {
		return fmt.Errorf("fail to MarshalAny")
	}
	msg := crypto.Message{DataMsg: any}
	msg.From = c.Net.publicKey
	msg.To = c.peerID
	msg.Timestamp = time.Now().UnixNano()
	msg.Keygen = c.Net.selfKeygen
	data, err := proto.Marshal(&msg)
	if err != nil {
		return fmt.Errorf("fail to MarshalAny")
	}
	msg.Sign = c.Net.selfKey.Sign(data)
	if msg.Sign == nil {
		return fmt.Errorf("fail to sign")
	}
	data, _ = proto.Marshal(&msg)
	l := int64(len(data))
	if l > messageLengthLimit {
		return fmt.Errorf("data too long:%d", l)
	}
	stream, err := c.session.OpenStream()
	if err != nil {
		return err
	}
	log.Printf("Send msg. From:%x, To:%x, Timestamp:%d\n", msg.From, msg.To, msg.Timestamp)

	//defer stream.Close()
	stream.SetDeadline(time.Now().Add(10 * time.Second))

	ld := make([]byte, binary.MaxVarintLen64)
	binary.PutVarint(ld, l)
	_, err = stream.Write(ld)
	if err != nil {
		return err
	}
	_, err = stream.Write(data)
	log.Println("send data length:", l, ", stream id:", stream.ID())
	return err
}

// GetRemoteAddress get remote address. kcp://publicKey@host:port
func (c *PeerSession) GetRemoteAddress() string {
	addr := fmt.Sprintf("kcp://%x@%s", c.peerID, c.remoteAddr)
	return addr
}