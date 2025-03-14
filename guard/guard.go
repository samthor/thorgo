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
	stop     func() bool
	seq      int
	sessions map[*guardSession[Token, Key]]struct{}
}

func (g *guardImpl[Token, Key]) ProvideToken(t Token, expiry time.Time) {
	duration := time.Until(expiry)

	g.tokenLock.Lock()
	defer g.tokenLock.Unlock()

	td, ok := g.tokens[t]
	if !ok {
		td = &tokenData[Token, Key]{
			sessions: map[*guardSession[Token, Key]]struct{}{},
		}
		g.tokens[t] = td
	} else {
		if !td.stop() {
			td.seq++
		}
	}

	expectedSeq := td.seq
	timer := time.AfterFunc(duration, func() {
		g.expireToken(t, expectedSeq)
	})
	td.stop = timer.Stop
}

func (g *guardImpl[Token, Key]) expireToken(t Token, expectedSeq int) {
	// find all sessions using us, remove us
	// if any of those sessions have zero tokens, nuke them

	g.tokenLock.Lock()

	td := g.tokens[t]
	if td == nil {
		panic("unknown token expired")
	}
	if td.seq != expectedSeq {
		return // we expired right as we were being stoppewd
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
			use, err := g.check(g.ctx, gs.key, tokens)
			if err != nil {
				gs.lock.Lock()
				gs.err = err
				gs.lock.Unlock()
				// use should be nil with err; make sure anyway, installSession will close our ch
				use = nil
			}
			g.installSession(gs, use)
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
		}

		g.tokenLock.Unlock()
	}

	if len(valid) == 0 {
		// not referenced anywhere, we can close
		close(gs.ch)
	}
}

type Session[Token any] interface {
	Err() error
	TokenCh() <-chan Token
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

	lock sync.Mutex
	err  error
}

func (g *guardSession[Token, Key]) Err() error {
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.err
}

func (g *guardSession[Token, Key]) TokenCh() <-chan Token {
	return g.ch
}
