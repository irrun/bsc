package miner

type flashbotsData struct {
	isFlashbots      bool
	queue            chan *task
	maxMergedBundles int
	algoType         AlgoType
}
