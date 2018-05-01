package main

import (
	"github.com/it-chain/it-chain-Engine/legacy/service"
	"github.com/it-chain/it-chain-Engine/legacy/domain"
	"github.com/it-chain/it-chain-Engine/legacy/network/comm"
	pb "github.com/it-chain/it-chain-Engine/legacy/network/protos"
	"io"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"github.com/it-chain/it-chain-Engine/legacy/auth"
	"net"
	"google.golang.org/grpc/reflection"
	"log"
	"google.golang.org/grpc"
	"github.com/it-chain/it-chain-Engine/legacy/network/comm/publisher"
	"time"
	"github.com/urfave/cli"
	"os"
	"strings"
)

var View = &domain.View{
	ID:"127.0.0.1:4444",
	LeaderID: "127.0.0.1:4444",
	PeerID: []string{"127.0.0.1:5555","127.0.0.1:6666","127.0.0.1:7777","127.0.0.1:4444"},
}

//pbft testing code
type Node struct {
	myInfo            *domain.Peer
	ip                string
	port              string
	consensusService  service.ConsensusService
	view              *domain.View
	connectionManager comm.ConnectionManager
	blockService      service.BlockService
	peerService       service.PeerService
	messagePublisher  publisher.MessagePublisher
	perviousBlock     *domain.Block
}

func NewNode(peerInfo *domain.Peer) *Node{

	node := &Node{}
	node.myInfo = peerInfo

	node.perviousBlock = nil

	crypto, err := auth.NewCrypto("./sample/pbft/"+node.myInfo.GetEndPoint())
	_, pub ,err := crypto.GenerateKey(&auth.RSAKeyGenOpts{})

	if err !=nil{
		log.Println(err)
	}

	node.myInfo.PubKey = pub.SKI()

	//log.Println(node.myInfo.PubKey)

	if err != nil{
		panic("fail to create keys")
	}

	connectionManager := comm.NewConnectionManagerImpl(crypto)
	//consensusService
	consensusService := service.NewPBFTConsensusService(connectionManager,nil,node.myInfo.PeerID)

	//peerService
	peerTable,err := domain.NewPeerTable(node.myInfo)

	if err != nil{
		panic("error set peertable")
	}

	peerService := service.NewPeerServiceImpl(peerTable,connectionManager)

	eventBatcher := service.NewBatchService(time.Second*5,peerService.BroadCastPeerTable,false)
	eventBatcher.Add("push peerTable")

	//publisher.NewMessagePublisher(domain.,crypto)
	node.consensusService = consensusService
	node.peerService = peerService
	node.connectionManager = connectionManager
	node.view = View

	return node
}

func (s *Node) Stream(stream pb.MessageService_StreamServer) (error) {

	for {
		envelope,err := stream.Recv()

		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		message := &pb.Message{}
		err = proto.Unmarshal(envelope.Payload,message)

		if err != nil{
			log.Println(err)
		}

		//log.Println("Received Envelop:",envelope)

		if message.GetConsensusMessage() != nil{
			log.Println("Consensus Message")
			//pcm := message.GetConsensusMessage()
			//consensusMessage := domain.FromConsensusProtoMessage(*pcm)
			//consensusMessage.TimeStamp = time.Now()
			//log.Println(consensusMessage)

			outterMessage := comm.OutterMessage{Message:message}
			s.consensusService.ReceiveConsensusMessage(outterMessage)

			continue
		}

		if message.GetPeerTable() != nil{
			pt := message.GetPeerTable()
			peerTable := domain.FromProtoPeerTable(*pt)
			s.peerService.UpdatePeerTable(*peerTable)
			continue
		}
	}
}

func (s *Node) Ping(ctx context.Context, in *pb.Empty) (*pb.Empty, error) {
	return &pb.Empty{}, nil
}

func (s *Node) GetPeer(context.Context, *pb.Empty) (*pb.Peer, error){

	pp := domain.ToProtoPeer(*s.myInfo)

	log.Println(pp)
	return pp,nil
}

func (s *Node) StartConsensus(context.Context, *pb.Empty) (*pb.Empty, error){

	log.Println("start consensus!!")

	var block *domain.Block

	if s.perviousBlock == nil{
		block = domain.CreateNewBlock(nil,s.myInfo.PeerID)
	}else{
		block = domain.CreateNewBlock(s.perviousBlock,s.myInfo.PeerID)
	}

	s.consensusService.StartConsensus(View,block)

	return &pb.Empty{},nil
}

func (s *Node) listen(){

	lis, err := net.Listen("tcp", s.myInfo.GetEndPoint())

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	pb.RegisterMessageServiceServer(server, s)
	pb.RegisterTestConsensusServiceServer(server,s)
	pb.RegisterPeerServiceServer(server,s)
	// Register reflection service on gRPC server.
	reflection.Register(server)

	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
		server.Stop()
		lis.Close()
	}
}

func (s *Node) RequestPeer(address string) *pb.Peer{
	log.Println("request peer Information to boot peer:",address)

	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	defer conn.Close()
	c := pb.NewPeerServiceClient(conn)

	peer, err := c.GetPeer(context.Background(), &pb.Empty{})

	if err != nil {
		log.Println("could not greet: %v", err)
	}

	log.Println("recevied peer Info:",peer)

	return peer
}

func main(){

	app := cli.NewApp()

	var myAddress = ""
	var bootAddress = ""

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "IP, ip",
			Usage:       "hostIP:port",
			Destination: &myAddress,
		},
		cli.StringFlag{
			Name:        "BootIP, bi",
			Usage:       "hostIP:port",
			Destination: &bootAddress,
		},
	}

	app.Action = func(c *cli.Context) error {

		address := strings.Split(myAddress,":")

		peer := &domain.Peer{}
		peer.PeerID = myAddress
		peer.IpAddress = address[0]
		peer.Port = address[1]

		log.Println(myAddress)

		node := NewNode(peer)

		if bootAddress != ""{
			log.Println("searching boot peer...")
			p := node.RequestPeer(bootAddress)
			bootPeer := domain.FromProtoPeer(*p)
			node.peerService.AddPeer(bootPeer)
		}

		node.listen()

		return nil
	}

	app.Run(os.Args)
}


