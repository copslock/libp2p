package libp2p

import (
	"fmt"
	"github.com/lengzhao/libp2p/message"
	"log"
	"net/url"
	"testing"
	"time"
)

func TestAddress(t *testing.T) {
	addrStr := "kcp://pubKey@0.0.0.0:3000"
	u, err := url.Parse(addrStr)
	if err != nil {
		log.Println("error address:", addrStr, err)
		return
	}
	fmt.Println("Scheme", u.Scheme)
	fmt.Println("User", u.User)
	fmt.Println("Username", u.User.Username())
	p, _ := u.User.Password()
	fmt.Println("Password", p)
	fmt.Println("Host", u.Host)
	if u.Scheme != "kcp" || u.User.Username() != "pubKey" {
		t.Error("scheme or username error.")
	}
	if u.Host != "0.0.0.0:3000" {
		t.Error("Host error.")
	}
}

func TestNetwork_Listen(t *testing.T) {
	n1 := NewNetwork("kcp://127.0.0.1:3000")
	go n1.Listen()
	time.Sleep(1 * time.Second)
	fmt.Println("listen address:", n1.GetAddress())
	if !n1.started {
		t.Error("fail to listen")
	}
	n1.Close()
	if n1.started {
		t.Error("fail to close")
	}
}

type PingPlugin struct {
	*Plugin
	pingCount int
	pongCount int
}

func (state *PingPlugin) Receive(ctx *PluginContext) error {
	switch msg := ctx.GetMessage().(type) {
	case *message.DhtPong:
		fmt.Printf("Pong <%s> %s\n", ctx.GetRemoteID(), msg.String())
		state.pongCount++
	case *message.DhtPing:
		fmt.Printf("ping <%s> %s\n", ctx.GetRemoteID(), msg.String())
		state.pingCount++
		ctx.Reply(new(message.DhtPong))
	}

	return nil
}

func TestNetwork_NewSession(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	n1 := NewNetwork("kcp://127.0.0.1:3000")
	n2 := NewNetwork("kcp://127.0.0.1:3001")
	plug := new(PingPlugin)
	n1.AddPlugin(plug)
	n2.AddPlugin(plug)
	go n1.Listen()
	go n2.Listen()
	time.Sleep(1 * time.Second)
	n1Addr := n1.GetAddress()
	// n2->n1
	session := n2.NewSession(n1Addr)
	if session == nil {
		t.Error("fail to new session from n2->n1. n1.address:", n1Addr)
		return
	}
	fmt.Println("n2->n1. ", n2.GetAddress(), " --> ", n1Addr)

	err := session.Send(&message.DhtPing{})
	if err != nil {
		t.Error("fail to send msg.", err)
		return
	}
	time.Sleep(1 * time.Second)
	//session.Close()
	if plug.pingCount != 1 {
		t.Errorf("hope server receive Ping message.%d\n", plug.pingCount)
	}
	if plug.pongCount != 1 {
		t.Errorf("hope server receive Pong message.%d\n", plug.pongCount)
	}

	// n1->n2
	session = n1.NewSession(n2.GetAddress())
	if session == nil {
		t.Error("fail to new session from n2->n1. n1.address:", n1Addr)
		return
	}
	fmt.Println("n2->n1. ", n2.GetAddress(), " --> ", n1Addr)

	err = session.Send(&message.DhtPing{})
	if err != nil {
		t.Error("fail to send msg.", err)
		return
	}
	time.Sleep(1 * time.Second)
	if plug.pingCount != 2 {
		t.Errorf("hope server receive Ping message.%d\n", plug.pingCount)
	}
	if plug.pongCount != 2 {
		t.Errorf("hope server receive Pong message.%d\n", plug.pongCount)
	}
	session.Close()
	n1.Close()
	n2.Close()
	//t.Error("stop")
}

func TestNetwork_NewSession2(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	n1 := NewNetwork("kcp://127.0.0.1:3000")
	n2 := NewNetwork("kcp://127.0.0.1:3001")
	plug := new(PingPlugin)
	n1.AddPlugin(plug)
	n2.AddPlugin(plug)
	go n1.Listen()
	go n2.Listen()
	time.Sleep(1 * time.Second)
	n1Addr := n1.GetAddress()
	session := n2.NewSession(n1Addr)
	if session == nil {
		t.Error("fail to new session from n2->n1. n1.address:", n1Addr)
		return
	}
	fmt.Println("n2->n1. ", n2.GetAddress(), " --> ", n1Addr)

	err := session.Send(&message.DhtPing{})
	if err != nil {
		t.Error("fail to send msg.", err)
		return
	}
	time.Sleep(1 * time.Second)
	session.Close()
	if plug.pingCount != 1 {
		t.Errorf("hope server receive Ping message.%d\n", plug.pingCount)
	}
	if plug.pongCount != 1 {
		t.Errorf("hope server receive Pong message.%d\n", plug.pongCount)
	}

	// the session of n2->n1 is closed,time wait
	time.Sleep(6 * time.Second)
	// reconnect
	session = n1.NewSession(n2.GetAddress())
	if session == nil {
		t.Error("fail to new session from n2->n1. n1.address:", n1Addr)
		return
	}
	fmt.Println("n2->n1. ", n2.GetAddress(), " --> ", n1Addr)

	err = session.Send(&message.DhtPing{})
	if err != nil {
		t.Error("fail to send msg.", err)
		return
	}
	time.Sleep(1 * time.Second)
	if plug.pingCount != 2 {
		t.Errorf("hope server receive Ping message.%d\n", plug.pingCount)
	}
	if plug.pongCount != 2 {
		t.Errorf("hope server receive Pong message.%d\n", plug.pongCount)
	}
	session.Close()
	n1.Close()
	n2.Close()
	//t.Error("stop")
}
