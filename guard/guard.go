package guard

import (
	"context"
	"sync"
	"time"
)

type guardImpl[Token comparable, Key any] struct {
	ctx   context.Context
	check CheckFunc[Token, Key]

	tokenLock sync.RWMutex
	tokens    map[Token]*tokenData[Token, Key]
}

type tokenData[Token comparable, Key any] struct {
	sessions map[*guardSession[Token, Key]]struct{}

	refreshCh  chan struct{}   // closed when a new shutdownCh is available
	shutdownCh <-chan struct{} // closed by user
}

func (g *guardImpl[Token, Key]) ProvideToken(t Token, shutdown <-chan struct{}) {
	g.tokenLock.Lock()

	td, ok := g.tokens[t]
	if ok {
		if shutdown != td.shutdownCh {
			close(td.refreshCh)
			td.refreshCh = make(chan struct{})
			td.shutdownCh = shutdown
		}
		g.tokenLock.Unlock()
		return
	}

	td = &tokenData[Token, Key]{
		sessions:   map[*guardSession[Token, Key]]struct{}{},
		refreshCh:  make(chan struct{}),
		shutdownCh: shutdown,
	}
	g.tokens[t] = td

	go func() {
		for {
			shutdown := td.shutdownCh
			refresh := td.refreshCh
			g.tokenLock.Unlock()

			select {
			case <-shutdown:
				g.tokenLock.Lock()
				if shutdown != td.shutdownCh {
					continue // someone changed us (this is dealing with a race): restart
				}

				g.expireToken(t)
				return

			case <-refresh:
				// we got a new td.shutdownCh, grab it under lock
			}
			g.tokenLock.Lock()
		}
	}()

}

func (g *guardImpl[Token, Key]) ProvideTokenExpiry(t Token, expiry time.Time) {
	shutdownCh := make(chan struct{})
	duration := time.Until(expiry)

	time.AfterFunc(duration, func() {
		close(shutdownCh)
	})

	g.ProvideToken(t, shutdownCh)
}

// expireLock MUST be called under lock, and it releases the lock.
func (g *guardImpl[Token, Key]) expireToken(t Token) {
	// find all sessions using us, remove us
	// if any of those sessions have zero tokens, nuke them

	td := g.tokens[t]
	if td == nil {
		panic("unknown token expired")
	}

	var invalidSessions []*guardSession[Token, Key]

	for gs := range td.sessions {
		delete(gs.tokens, t)
		if len(gs.tokens) > 0 {
			continue // fine for now
		}

		// we're out of tokens for this session: need to ask if there's more?
		invalidSessions = append(invalidSessions, gs)
		delete(td.sessions, gs) // we MUST return this later if valid
	}
	delete(g.tokens, t)

	if len(invalidSessions) == 0 {
		g.tokenLock.Unlock()
		return
	}

	// take slice before we release lock
	tokens := make([]Token, 0, len(g.tokens))
	for t := range g.tokens {
		tokens = append(tokens, t)
	}
	g.tokenLock.Unlock()

	for _, gs := range invalidSessions {
		go func() {
			// only we have "gs" reference here, the lock is just for user-facing stuff
			gs.lock.Lock()
			if gs.stopped {
				gs.lock.Unlock()
				return // don't install again
			}

			gs.lock.Unlock()
			// we unlock here because g.check might take "time", and the user of gs might still want to look at it
			use, err := g.check(g.ctx, gs.key, tokens)
			gs.lock.Lock()
			if err != nil {
				gs.err = err
				// use should be nil with err; make sure anyway, installSession will close our ch
				use = nil
			}

			// check stopped agian
			if !gs.stopped {
				g.installSession(gs, use)
			}
			gs.lock.Unlock()
		}()
	}
}

// installSession must NOT be held under tokenLock.
func (g *guardImpl[Token, Key]) installSession(gs *guardSession[Token, Key], use []Token) {
	valid := make([]Token, 0, len(use))

	if len(use) > 0 {
		g.tokenLock.Lock()

		for _, token := range use {
			td, ok := g.tokens[token]
			if !ok {
				// handles both expiry _and_ caller gave us a weird token
				continue
			}
			td.sessions[gs] = struct{}{}
			valid = append(valid, token)

			// won't block in RunSession, since chan is at least len(use)
			// in running session behavior, the tokens MUST be consumed
			gs.ch <- token

			// record use
			gs.tokens[token] = struct{}{}
		}

		g.tokenLock.Unlock()
	}

	if len(valid) == 0 && !gs.stopped {
		// not referenced anywhere, we can close
		gs.stopped = true
		close(gs.ch)
	}
}

func (g *guardImpl[Token, Key]) RunSession(key Key) (Session[Token], error) {
	// take slice that we can use (CheckSession might take a while)
	g.tokenLock.RLock()
	tokens := make([]Token, 0, len(g.tokens))
	for t := range g.tokens {
		tokens = append(tokens, t)
	}
	g.tokenLock.RUnlock()

	var use []Token
	if len(tokens) > 0 {
		var err error
		use, err = g.check(g.ctx, key, tokens)
		if err != nil {
			return nil, err
		}
	}

	// buffer chan for the use size: that way we can send them all here
	ch := make(chan Token, len(use))
	gs := &guardSession[Token, Key]{
		key:    key,
		tokens: map[Token]struct{}{},
		ch:     ch,
	}
	g.installSession(gs, use)

	return gs, nil
}

func NewGuard[Token comparable, Key any](ctx context.Context, check CheckFunc[Token, Key]) Guard[Token, Key] {
	return &guardImpl[Token, Key]{
		check:  check,
		ctx:    ctx,
		tokens: map[Token]*tokenData[Token, Key]{},
	}
}

type guardSession[Token comparable, Key any] struct {
	key    Key
	tokens map[Token]struct{}
	ch     chan Token

	lock    sync.Mutex
	stopped bool
	err     error
}

func (gs *guardSession[Token, Key]) Err() error {
	gs.lock.Lock()
	defer gs.lock.Unlock()
	return gs.err
}

func (gs *guardSession[Token, Key]) TokenCh() <-chan Token {
	return gs.ch
}

func (gs *guardSession[Token, Key]) Stop() {
	go func() {
		for range gs.ch {
			// drain ch (maybe the caller gave up listening to Stop)
		}
	}()

	gs.lock.Lock()
	defer gs.lock.Lock()
	if !gs.stopped {
		gs.stopped = true
		close(gs.ch)
	}
}
