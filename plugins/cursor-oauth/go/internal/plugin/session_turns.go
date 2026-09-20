package plugin

import "sync"

type sessionTurnKey struct {
	account string
	model   string
	session string
}

type sessionTurnLock struct {
	mu   sync.Mutex
	refs int
}

type sessionTurnLocks struct {
	mu    sync.Mutex
	locks map[sessionTurnKey]*sessionTurnLock
}

func newSessionTurnLocks() *sessionTurnLocks {
	return &sessionTurnLocks{locks: make(map[sessionTurnKey]*sessionTurnLock)}
}

func (locks *sessionTurnLocks) acquire(key sessionTurnKey) func() {
	locks.mu.Lock()
	turn := locks.locks[key]
	if turn == nil {
		turn = &sessionTurnLock{}
		locks.locks[key] = turn
	}
	turn.refs++
	locks.mu.Unlock()
	turn.mu.Lock()
	return func() {
		turn.mu.Unlock()
		locks.mu.Lock()
		turn.refs--
		if turn.refs == 0 {
			delete(locks.locks, key)
		}
		locks.mu.Unlock()
	}
}
