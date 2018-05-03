package bifrost

import (
	"context"
	"errors"
	"log"
	"net"

	"sync"

	"encoding/json"

	"github.com/it-chain/bifrost/pb"
	"github.com/it-chain/heimdall/key"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Server struct {
	onConnectionHandler OnConnectionHandler
	onErrorHandler      OnErrorHandler
	priKey              key.PriKey
	pubKey              key.PubKey
	ip                  string
}

func (s Server) BifrostStream(streamServer pb.StreamService_BifrostStreamServer) error {
	//1. RquestPeer를 통해 나에게 Stream연결을 보낸 ConnInfo의정보를 확인
	//2. ConnInfo의정보를정보를 기반으로 Connection을 생성
	//3. 생성완료후 OnConnectionHandler를 통해 처리한다.

	envelope := &pb.Envelope{Type: pb.Envelope_REQUEST_PEERINFO}

	err := streamServer.Send(envelope)

	if err != nil {
		return err
	}

	if m, err := recvWithTimeout(3, streamServer); err == nil {

		wg := sync.WaitGroup{}
		wg.Add(1)

		valid, peerInfo := validateRequestPeerInfo(m)

		if !valid {
			return errors.New("fail to validate request peer info")
		}

		envelope, err := buildRequestPeerInfo(s.ip, s.pubKey)

		if err != nil {
			return errors.New("fail to build info")
		}

		if err = streamServer.Send(envelope); err != nil {
			return err
		}

		_, cf := context.WithCancel(context.Background())
		streamWrapper := NewServerStreamWrapper(streamServer, cf)

		pubKey, err := ByteToPubKey(peerInfo.Pubkey, peerInfo.KeyGenOpt, peerInfo.KeyType)

		//todo mux를 외부에서 세팅후에 넣어주는 부분 추가 해야함ㅎ
		conn, err := NewConnection(peerInfo.ip, s.priKey, pubKey, streamWrapper, nil)

		defer conn.Close()

		go func() {
			if err = conn.Start(); err != nil {
				conn.Close()
				wg.Done()
			}
		}()

		s.onConnectionHandler(conn)

		wg.Wait()
	}

	return nil
}

func validateRequestPeerInfo(envelope *pb.Envelope) (bool, *PeerInfo) {

	if envelope.GetType() != pb.Envelope_REQUEST_PEERINFO {
		log.Printf("Invaild message type")
		return false, nil
	}

	log.Printf("Received payload [%s]", envelope.Payload)

	var peerInfo *PeerInfo

	err := json.Unmarshal(envelope.Payload, peerInfo)

	if err != nil {
		return false, nil
	}

	return true, peerInfo
}

type OnConnectionHandler func(connection Connection)
type OnErrorHandler func(err error)

func NewServer(key KeyOpts) *Server {
	return &Server{
		priKey: key.priKey,
		pubKey: key.pubKey,
	}
}

func (s Server) OnConnection(handler OnConnectionHandler) {

	if handler == nil {
		return
	}

	s.onConnectionHandler = handler
}

func (s Server) OnError(handler OnErrorHandler) {

	if handler == nil {
		return
	}

	s.onErrorHandler = handler
}

func (s Server) Listen(ip string) {

	lis, err := net.Listen("tcp", ip)

	defer lis.Close()

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	g := grpc.NewServer()

	defer g.Stop()
	pb.RegisterStreamServiceServer(g, s)
	reflection.Register(g)

	log.Println("Listen... on: [%s]", ip)
	if err := g.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
		g.Stop()
		lis.Close()
	}
}
