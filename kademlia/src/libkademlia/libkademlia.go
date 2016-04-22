package libkademlia

// Contains the core kademlia type. In addition to core state, this type serves
// as a receiver for the RPC methods, which is required by that package.

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
)

const (
	alpha = 3
	b     = 8 * IDBytes
	k     = 20
)


type KVpair struct {
	key 	ID
	value	[]byte
}
// Kademlia type. You can put whatever state you need in this.
type Kademlia struct {
	NodeID      	ID
	SelfContact 	Contact
	
	RouteTable  	[]K_Buckets			// 0 based 
	DataStore		map[ID][]byte
	
	RTManagerChan   chan Contact
	DataStoreChan	chan KVpair	
	SearchKeyChan	chan ID
}

func NewKademliaWithId(laddr string, nodeID ID) *Kademlia {
	k := new(Kademlia)
	k.NodeID = nodeID

	// TODO: Initialize other state here as you add functionality.
	// !!!initial 160 lists, go updatethread,initial channel
	k.RouteTable = make([]K_Buckets, b)
	for i := 0; i < b; i++ {
		k.RouteTable[i] = *NewKBuckets(20, i)
	}
	
	k.RTManagerChan = make(chan Contact)
	k.DataStoreChan = make(chan KVpair)
	k.DataStore = make(map[ID][]byte)
	
	// Set up RPC server
	// NOTE: KademliaRPC is just a wrapper around Kademlia. This type includes
	// the RPC functions.

	s := rpc.NewServer()
	s.Register(&KademliaRPC{k})
	hostname, port, err := net.SplitHostPort(laddr)
	if err != nil {
		return nil
	}
	
	fmt.Printf("%d: begin start server", k.SelfContact.Port)
	s.HandleHTTP(rpc.DefaultRPCPath + port,
		rpc.DefaultDebugPath + port)
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Fatal("Listen: ", err)
	}
		// handle update
	go k.UpdateHandler()

	// Run RPC server forever.
	go http.Serve(l, nil)
	
	// Add self contact
	hostname, port, _ = net.SplitHostPort(l.Addr().String())
	port_int, _ := strconv.Atoi(port)
	ipAddrStrings, err := net.LookupHost(hostname)
	var host net.IP
	for i := 0; i < len(ipAddrStrings); i++ {
		host = net.ParseIP(ipAddrStrings[i])
		if host.To4() != nil {
			break
		}
	}
	k.SelfContact = Contact{k.NodeID, host, uint16(port_int)}
	
	fmt.Printf("%d: create new kademlia node ", k.SelfContact.Port)
	return k
}

func NewKademlia(laddr string) *Kademlia {
	return NewKademliaWithId(laddr, NewRandomID())
}

type ContactNotFoundError struct {
	id  ID
	msg string
}

func (e *ContactNotFoundError) Error() string {
	return fmt.Sprintf("%x %s", e.id, e.msg)
}

// find the left's bucket num containing right
func FindBucketNum(lhs ID, rhs ID) int {
	distance := lhs.Xor(rhs)
	//fmt.Println(distance.PrefixLen())
	return b - distance.PrefixLen() - 1
}

// Update my RouteTable
func (k *Kademlia) UpdateRouteTable(c *Contact) {
	num := FindBucketNum(k.NodeID, c.NodeID) 
	fmt.Printf("%d: insert num: %d \n", k.SelfContact.Port, num)

	l := k.RouteTable[num]								// list should contain c

	_, erro := k.FindContact(c.NodeID)
	if erro == nil{
		l.MoveToTail(c)
	} else {
		if !l.CheckFull() {
			l.AddTail(c)
			fmt.Println(l.bucket.Len())
		} else {
			head := l.GetHead()
			_, erro := k.DoPing(head.Host, head.Port)				//erro == nil, still active
			if erro != nil {
				l.RemoveHead()
				l.AddTail(c)
			}
		} 
	}
}

func (k *Kademlia) UpdateDataStore(p *KVpair)  {
	k.DataStore[p.key] = p.value
}

// one thread running Ro..Handler for thread safe
func (k *Kademlia) UpdateHandler() {
	fmt.Printf("%d: starting UpdateHandler \n", k.SelfContact.Port)
	for {
			select {
				case c := <- k.RTManagerChan:
					fmt.Printf("%d: before update RT \n", k.SelfContact.Port)
					k.UpdateRouteTable(&c)
				case p := <- k.DataStoreChan:
					fmt.Printf("%d: before DataStore \n", k.SelfContact.Port)
					k.UpdateDataStore(&p)
				// case key := <- k.SearchKeyChan:	
				default:
					continue
			}
	}
}

func (k *Kademlia) FindContact(nodeId ID) (*Contact, error) {
	// TODO: Search through contacts, find specified ID
	// Find contact with provided ID
	if nodeId == k.SelfContact.NodeID {
		return &k.SelfContact, nil
	} else {
		num := FindBucketNum(k.NodeID, nodeId)			//find number of list
		fmt.Printf("%d: find num: %d\n", k.SelfContact.Port, num)
		l := k.RouteTable[num].bucket
		fmt.Println(l.Len())
		for e := l.Front(); e != nil; e = e.Next() {
			fmt.Println("FIND...")
			if e.Value.(* Contact).NodeID.Equals(nodeId) {
				fmt.Printf("%d: successful find contact 2: \n", k.SelfContact.Port)
				return e.Value.(* Contact), nil
			}
		}
	}
	return nil, &ContactNotFoundError{nodeId, "Not found"}
}

type CommandFailed struct {
	msg string
}

func (e *CommandFailed) Error() string {
	return fmt.Sprintf("%s", e.msg)
}

func (k *Kademlia) DoPing(host net.IP, port uint16) (*Contact, error) {
	// TODO: Implement
	ping := PingMessage{k.SelfContact, NewRandomID()}
	pong := new(PongMessage)
	
	//client, err := rpc.DialHTTPPath("tcp", host.String() + ":" + strconv.Itoa(int (port)),
	//	                 rpc.DefaultRPCPath + strconv.Itoa(int (port)))
	firstPeerStr := host.String()+ ":" + strconv.Itoa(int (port))
	//client, err := rpc.DialHTTP("tcp", firstPeerStr)
	client, err := rpc.DialHTTPPath("tcp", firstPeerStr, rpc.DefaultRPCPath + strconv.Itoa(int(port)))
	fmt.Printf("%d: finish dialhttpath: \n", k.SelfContact.Port)
	//client, err := rpc.DialHTTP("tcp", host.String()+":"+strconv.FormatInt(int64(port), 10))
	if err != nil {
		fmt.Printf("%d: connection failed: \n", k.SelfContact.Port)
		//log.Fatal("dialing:", err)
		return nil, &CommandFailed{
		"Unable to ping " + fmt.Sprintf("%s:%v", host.String(), port)}
	}

	err = client.Call("KademliaRPC.Ping", ping, &pong)					// call remote server
    defer func() {
		client.Close()
	} ()
	
	fmt.Printf("%d: finish remote ping: \n", k.SelfContact.Port)
	if err == nil {
		fmt.Printf("%d: call succesfull: \n", k.SelfContact.Port)
		fmt.Printf("%d: pong sender port: %d \n", k.SelfContact.Port, pong.Sender.Port)

		k.RTManagerChan <- pong.Sender
		//c1 := <- k.RTManagerChan
		//		fmt.Printf("%d: channel %d \n", k.SelfContact.Port, c1.Port)
		fmt.Printf("%d: push to channel \n", k.SelfContact.Port)
		return &(pong.Sender), nil
	} else {
		return nil, &CommandFailed{
		"Unable to ping " + fmt.Sprintf("%s:%v", host.String(), port)}
	}
	
}

func (k *Kademlia) DoStore(contact *Contact, key ID, value []byte) error {
	// TODO: Implement
	
	storeReq := StoreRequest{k.SelfContact, NewRandomID(), key, value}
	storeRes := new(StoreResult)
	
	portStr := strconv.Itoa(int (contact.Port))
	firstPeerStr := contact.Host.String()+ ":" + portStr
	
	client, err := rpc.DialHTTPPath("tcp", firstPeerStr, rpc.DefaultRPCPath + portStr)

	if err != nil {
		fmt.Printf("%d: connection failed: \n", k.SelfContact.Port)
		//log.Fatal("dialing:", err)
		return &CommandFailed{
		"Unable to ping " + fmt.Sprintf("%s:%v", contact.Host.String(), contact.Port)}
	}
	
	err = client.Call("KademliaRPC.Store", storeReq, &storeRes)
	defer func() {
		client.Close()
	} ()
	
	if storeRes.Err != nil {
		return storeRes.Err
	}
	
	if storeReq.MsgID.Equals(storeRes.MsgID) {
		k.RTManagerChan <- *contact
	}
	
	return nil	
}

func (k *Kademlia) DoFindNode(contact *Contact, searchKey ID) ([]Contact, error) {
	// TODO: Implement
	req := FindNodeRequest{k.SelfContact, NewRandomID(), searchKey}
	res := new(FindNodeResult)
	
	portStr := strconv.Itoa(int (contact.Port))
	firstPeerStr := contact.Host.String()+ ":" + portStr
	
	client, err := rpc.DialHTTPPath("tcp", firstPeerStr, rpc.DefaultRPCPath + portStr)

	if err != nil {
		fmt.Printf("%d: connection failed: \n", k.SelfContact.Port)
		//log.Fatal("dialing:", err)
		return nil, &CommandFailed{
		"Unable to FindNode " + fmt.Sprintf("%s:%v", contact.Host.String(), contact.Port)}
	}
	
	err = client.Call("KademliaRPC.FindNode", req, &res)
	defer func() {
		client.Close()
	} ()
	
	if res.Err != nil {
		return nil, res.Err
	}
	
	if req.MsgID.Equals(res.MsgID) {
		k.RTManagerChan <- *contact
	}
	
	return res.Nodes, nil
}

func (k *Kademlia) DoFindValue(contact *Contact,
	searchKey ID) (value []byte, contacts []Contact, err error) {
	// TODO: Implement
	return nil, nil, &CommandFailed{"Not implemented"}
}

func (k *Kademlia) LocalFindValue(searchKey ID) ([]byte, error) {
	// TODO: Implement
	
	// !! TODO: this is not thread safe implmentation
	if val, ok := k.DataStore[searchKey]; ok {
		return val, nil
	} 
	return []byte(""), &CommandFailed{"Value not exists"}
}

// For project 2!
func (k *Kademlia) DoIterativeFindNode(id ID) ([]Contact, error) {
	return nil, &CommandFailed{"Not implemented"}
}
func (k *Kademlia) DoIterativeStore(key ID, value []byte) ([]Contact, error) {
	return nil, &CommandFailed{"Not implemented"}
}
func (k *Kademlia) DoIterativeFindValue(key ID) (value []byte, err error) {
	return nil, &CommandFailed{"Not implemented"}
}

// For project 3!
func (k *Kademlia) Vanish(data []byte, numberKeys byte,
	threshold byte, timeoutSeconds int) (vdo VanashingDataObject) {
	return
}

func (k *Kademlia) Unvanish(searchKey ID) (data []byte) {
	return nil
}
