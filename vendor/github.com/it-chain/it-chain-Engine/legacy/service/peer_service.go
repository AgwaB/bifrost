package service

import "github.com/it-chain/it-chain-Engine/legacy/domain"

//peer 최상위 service
type PeerService interface{

	//peer table 조회
	GetPeerTable() *domain.PeerTable

	//peer info 찾기
	GetPeerByPeerID(peerID string) *domain.Peer

	//peer info
	PushPeerTable(peerIDs []string)

	//update peerTable
	UpdatePeerTable(peerTable domain.PeerTable)

	//Add peer
	AddPeer(Peer *domain.Peer)

	//Request Peer Info
	RequestPeer(ip string) (*domain.Peer ,error)

	BroadCastPeerTable(interface{})

	GetLeader() *domain.Peer

	SetLeader(peer *domain.Peer)
}