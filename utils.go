package main

import (
	"sync"
)

//Semaphore ...
type Semaphore struct {
	wg sync.WaitGroup
	ch chan int
}

//Init ...
func (s *Semaphore) Init(count int) {
	s.ch = make(chan int, count)
}

//Add ...
func (s *Semaphore) Add() {
	s.wg.Add(1)
	s.ch <- 1
}

//Done ...
func (s *Semaphore) Done() {
	<-s.ch
	s.wg.Done()
}

//Wait ...
func (s *Semaphore) Wait() {
	s.wg.Wait()
}

//NewSemaphonre create a Semaphore
func NewSemaphonre(count int) *Semaphore {
	s := &Semaphore{}
	s.Init(count)
	return s
}
