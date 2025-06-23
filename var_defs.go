package main

import "sync"

var mu sync.Mutex
var blockCache = make(map[string]*BlockCache)
