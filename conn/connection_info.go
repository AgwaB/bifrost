package conn

import (
	"errors"
	"regexp"
	"strings"

	"github.com/it-chain/heimdall"
	b58 "github.com/jbenet/go-base58"
)

type ID string

func FromRsaPubKey(key heimdall.RsaPublicKey) ID {
	encoded := b58.Encode(key.SKI())
	return ID(encoded)
}

func FromRsaPriKey(key heimdall.RsaPrivateKey) ID {
	pub, _ := key.PublicKey()
	return FromRsaPubKey(*pub)
}

func (id ID) String() string {
	return string(id)
}

type Address struct {
	IP string
}

func validIP4(ipAddress string) bool {
	ipAddress = strings.Trim(ipAddress, " ")

	re, _ := regexp.Compile(`^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$`)
	if re.MatchString(ipAddress) {
		return true
	}
	return false
}

//format should be xxx.xxx.xxx.xxx:xxxx
func ToAddress(ipv4 string) (Address, error) {

	valid := validIP4(ipv4)

	if !valid {
		return Address{}, errors.New("invalid IP4 format")
	}

	return Address{
		IP: ipv4,
	}, nil
}

type ConnenctionInfo struct {
	Id      ID
	Address Address
	PubKey  heimdall.RsaPublicKey
}

func NewConnenctionInfo(id ID, address Address, pubKey heimdall.RsaPublicKey) *ConnenctionInfo {
	return &ConnenctionInfo{
		Id:      id,
		Address: address,
		PubKey:  pubKey,
	}
}

type MyConnectionInfo struct {
	*ConnenctionInfo
	PriKey heimdall.RsaPrivateKey
}

func NewMyConnectionInfo(id ID, address Address, pubKey heimdall.RsaPublicKey, priKey heimdall.RsaPrivateKey) *MyConnectionInfo {

	return &MyConnectionInfo{
		ConnenctionInfo: NewConnenctionInfo(id, address, pubKey),
		PriKey:          priKey,
	}
}

func (myConnectionInfo MyConnectionInfo) GetPublicInfo() ConnenctionInfo {
	return *myConnectionInfo.ConnenctionInfo
}
