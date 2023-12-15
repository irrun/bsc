package miner

import (
	"errors"
	"strings"
)

type AlgoType int

const (
	ALGO_MEV_GETH AlgoType = iota
	ALGO_GREEDY
	ALGO_GREEDY_BUCKETS
	ALGO_GREEDY_MULTISNAP
	ALGO_GREEDY_BUCKETS_MULTISNAP
)

func (a AlgoType) String() string {
	switch a {
	case ALGO_GREEDY:
		return "greedy"
	case ALGO_GREEDY_MULTISNAP:
		return "greedy-multi-snap"
	case ALGO_MEV_GETH:
		return "mev-geth"
	case ALGO_GREEDY_BUCKETS:
		return "greedy-buckets"
	case ALGO_GREEDY_BUCKETS_MULTISNAP:
		return "greedy-buckets-multi-snap"
	default:
		return "unsupported"
	}
}

func AlgoTypeFlagToEnum(algoString string) (AlgoType, error) {
	switch strings.ToLower(algoString) {
	case ALGO_MEV_GETH.String():
		return ALGO_MEV_GETH, nil
	case ALGO_GREEDY_BUCKETS.String():
		return ALGO_GREEDY_BUCKETS, nil
	case ALGO_GREEDY.String():
		return ALGO_GREEDY, nil
	case ALGO_GREEDY_MULTISNAP.String():
		return ALGO_GREEDY_MULTISNAP, nil
	case ALGO_GREEDY_BUCKETS_MULTISNAP.String():
		return ALGO_GREEDY_BUCKETS_MULTISNAP, nil
	default:
		return ALGO_MEV_GETH, errors.New("algo not recognized")
	}
}
