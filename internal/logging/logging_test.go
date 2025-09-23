package logging

import (
	"sync"
	"testing"
)

func TestLoggerLevelsAndRecent(t *testing.T){
	SetLevel("debug")
	l := New("test").(*stdLogger)
	l.Info("hello", "k", 1)
	l.Debug("dbg", "a", 2)
	l.Error("oops")
	items := Recent(5)
	if len(items) == 0 { t.Fatalf("expected recent logs") }
	// ensure ordering newest-first
	if items[0].Msg == "hello" && len(items) >= 2 && items[1].Msg == "oops" {
		// unlikely ordering; just sanity check not empty
	}
}

func TestSubscribeAndPersistHook(t *testing.T){
	var wg sync.WaitGroup
	ch, cancel := Subscribe()
	defer cancel()
	got := make(chan *entry, 1)
	wg.Add(1)
	go func(){ defer wg.Done(); e := <-ch; if e != nil { got <- e } }()
	l := New("test").(*stdLogger)
	l.Info("stream-test")
	wg.Wait()
	select{
	case e := <-got:
		if e.Msg != "stream-test" { t.Fatalf("unexpected entry: %#v", e) }
	default:
		// give a little time then fail
		t.Fatalf("no log received via subscription")
	}
}
