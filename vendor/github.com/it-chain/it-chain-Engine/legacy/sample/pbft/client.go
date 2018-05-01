package main

import (
	"log"
	"google.golang.org/grpc"
	pb "github.com/it-chain/it-chain-Engine/legacy/network/protos"
	"context"
)

func main(){

	address := "127.0.0.1:4444"

	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	defer conn.Close()
	//c := pb.N
	//
	//_, err = c.StartConsensus(context.Background(), &pb.Empty{})
	//
	//if err != nil {
	//	log.Println("could not greet: %v", err)
	}
}
