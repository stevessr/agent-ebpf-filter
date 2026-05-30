package main

import (
	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/pb"
)

type BehaviorEmbedding = behavior.BehaviorEmbedding
type ClusterID = behavior.ClusterID
type EventCluster = behavior.EventCluster
type InstructionEmbedder = behavior.InstructionEmbedder
type Classifier = behavior.Classifier

var globalEmbedder = behavior.NewInstructionEmbedder()
var globalClassifier = &Classifier{}

func ClassifyBehavior(comm string, args []string) *pb.BehaviorClassification {
	return behavior.ClassifyBehavior(comm, args)
}
