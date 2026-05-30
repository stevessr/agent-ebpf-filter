package main

import (
	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/pb"
)

type Classifier = behavior.Classifier

var globalClassifier = &Classifier{}

func ClassifyBehavior(comm string, args []string) *pb.BehaviorClassification {
	return behavior.ClassifyBehavior(comm, args)
}
