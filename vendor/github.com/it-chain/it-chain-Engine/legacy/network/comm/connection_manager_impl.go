package comm

import (
	"sync"
	"github.com/it-chain/it-chain-Engine/common"
	"github.com/it-chain/it-chain-Engine/legacy/auth"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	pb "github.com/it-chain/it-chain-Engine/legacy/network/protos"
	"google.golang.org/grpc/peer"
	"github.com/pkg/errors"
	"github.com/it-chain/it-chain-Engine/legacy/network/comm/publisher"
	"github.com/it-chain/it-chain-Engine/legacy/network/comm/msg"
	"github.com/it-chain/it-chain-Engine/legacy/network/comm/conn"
)

var commLogger = common.GetLogger("connection_manager_impl.go")

type OnConnectionHandler func(conn conn.Connection, peer pb.Peer)

//Connection관리와 message-> Envelop검사를 수행한다.
type ConnectionManagerImpl struct {
	connectionMap       map[string]conn.Connection
	crpyto              auth.Crypto
	onConnectionHandler OnConnectionHandler
	msgPublisher        *publisher.MessagePublisher
	sync.RWMutex
}

func NewConnectionManagerImpl(crpyto auth.Crypto) ConnectionManager{
	return &ConnectionManagerImpl{
		connectionMap: make(map[string]conn.Connection),
		msgPublisher: publisher.NewMessagePublisher(crpyto),
		crpyto: crpyto,
	}
}

func (comm *ConnectionManagerImpl) SetOnConnectHandler(onConnectionHandler OnConnectionHandler){

	comm.Lock()
	defer comm.Unlock()

	if onConnectionHandler != nil{
		comm.onConnectionHandler = onConnectionHandler
	}
}

func (comm *ConnectionManagerImpl) Subscribe(name string, subfunc func(message msg.OutterMessage)){

	comm.Lock()
	defer comm.Unlock()

	comm.msgPublisher.AddSubscriber(name,subfunc)
}

func (comm *ConnectionManagerImpl) CreateStreamClientConn(connectionID string, ip string) error{

	//Peer의 connectionID로 connection을 연결
	_, ok := comm.connectionMap[connectionID]

	if ok{
		return nil
	}

	grpcConnection,err := conn.NewConnectionWithAddress(ip,false,nil)

	if err != nil{
		return err
	}

	ctx, cf := context.WithCancel(context.Background())
	client := pb.NewStreamServiceClient(grpcConnection)
	clientStream, err := client.Stream(ctx)

	//serverStream should be nil
	conn,err := conn.NewConnection(clientStream,nil,
		grpcConnection,client,comm.msgPublisher.ReceivedMessageHandle,connectionID,cf)

	if err != nil{
		return err
	}

	comm.Lock()
	comm.connectionMap[connectionID] = conn
	comm.Unlock()

	commLogger.Println("new conn:",connectionID, "are created")

	return nil
}

//todo close 어떻게 할지
func (comm *ConnectionManagerImpl) SendStream(message *pb.StreamMessage, successCallBack OnSuccess, errorCallBack OnError, connectionID string){

	//commLogger.Println("Sending data...")

	envelope, err := comm.signing(message)

	if err != nil{
		commLogger.Error("error: ",err)
		if errorCallBack != nil{
			errorCallBack(err)
		}
	}
	
	conn, ok := comm.connectionMap[connectionID]

	if ok{
		conn.Send(envelope,successCallBack, errorCallBack)
		//commLogger.Println("Sended Envelope:",envelope)
		//todo 어떤 error일 경우에 conn을 close 할지 정해야함
	}else{
		//todo 처리
	}
}

func (comm *ConnectionManagerImpl) Stop(){
	commLogger.Println("All connections are closing")
	for id, conn := range comm.connectionMap{
		conn.Close()
		delete(comm.connectionMap,id)
	}
}

func (comm *ConnectionManagerImpl) Close(connectionID string){

	conn, ok := comm.connectionMap[connectionID]

	if ok{
		commLogger.Println("Conn:",connectionID, "is closing")
		conn.Close()
		delete(comm.connectionMap,connectionID)
	}
}

func (comm *ConnectionManagerImpl) Size() int{
	return len(comm.connectionMap)
}

func (comm *ConnectionManagerImpl) Stream(stream pb.StreamService_StreamServer) (error) {
	//1. RquestPeer를 통해 나에게 Stream연결을 보낸 Peer의정보를 확인
	//2. Peer정보를 기반으로 Connection을 생성
	//3. 생성완료후 OnConnectionHandler를 통해 처리한다.

	//remoteAddress := extractRemoteAddress(stream)
	commLogger.Println("New conn requests are made")

	e := &pb.ConnectionEstablish{}
	message := &pb.StreamMessage{}
	message.Content = &pb.StreamMessage_ConnectionEstablish{
		ConnectionEstablish: e,
	}

	envelope, err := comm.signing(message)

	if err != nil{
		commLogger.Errorln(err.Error())
	}

	commLogger.Println("Sending conn establish message")

	err = stream.Send(envelope)

	if err != nil{
		commLogger.Errorln(err.Error())
	}

	commLogger.Println("Waiting response..")

	if m, err := stream.Recv(); err == nil {

		wg := sync.WaitGroup{}
		wg.Add(1)

		message,err := m.GetMessage()

		if err != nil{
			commLogger.Errorln("failed to unmarshal", err.Error())
		}

		if pp := message.GetPeer(); pp != nil{

			_, cf := context.WithCancel(context.Background())

			connectionID := pp.PeerID
			//todo handler 넣어주기
			commLogger.Println("creating new conn")
			conn,err := conn.NewConnection(nil,stream,
				nil,nil, comm.msgPublisher.ReceivedMessageHandle,connectionID,cf)


			if err != nil{
				return err
			}

			_, ok := comm.connectionMap[connectionID]

			if !ok{
				comm.Lock()
				comm.connectionMap[connectionID] = conn
				comm.Unlock()

				if comm.onConnectionHandler != nil{
					comm.onConnectionHandler(conn,*pp)
				}else{
					commLogger.Debugln("no onConnected Handler")
				}
			}else{
				commLogger.Debugln("already exist conn:",connectionID)
			}
		}

		wg.Wait()
	}

	commLogger.Println("Closing server stream")

	return nil
}

func extractRemoteAddress(stream pb.StreamService_StreamServer) string {
	var remoteAddress string
	p, ok := peer.FromContext(stream.Context())
	if ok {
		if address := p.Addr; address != nil {
			remoteAddress = address.String()
		}
	}
	return remoteAddress
}

func (comm *ConnectionManagerImpl) signing(message *pb.StreamMessage) (*pb.Envelope, error) {
	payload, err := proto.Marshal(message)

	if err != nil{
		return nil, errors.New("Marshal error: "+ err.Error())
	}

	_, pub, err  := comm.crpyto.GetKey()

	if err != nil{
		return nil, errors.New("Load key error: "+ err.Error())
	}

	sig, err  := comm.crpyto.Sign(payload,auth.DefaultRSAOption)

	if err != nil{
		return nil, errors.New("Signing error: "+ err.Error())
	}

	envelope := &pb.Envelope{}
	envelope.Payload = payload
	envelope.Pubkey = pub.SKI()
	envelope.Signature = sig

	return envelope, nil
}