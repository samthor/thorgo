package guard

import (
	"context"
	"sync"
	"time"
)

type guardImpl[Token comparable, Key any] struct {
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

	time.AfterFunc(duration, func() { close(shutdownCh) })

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

			select {
			case <-gs.derivedCtx.Done():
				return // don't install again, user marked us as done
			default:
			}

			use, err := g.check(gs.derivedCtx, gs.key, tokens)
			if err != nil || len(use) == 0 {
				gs.cancel(err)
				gs.Stop()
			} else {
				g.installSession(gs, use)
			}
		}()
	}
}

// installSession must NOT be held under tokenLock.
func (g *guardImpl[Token, Key]) installSession(gs *guardSession[Token, Key], use []Token) {
	var anyValid bool

	g.tokenLock.Lock()

	for _, token := range use {
		td, ok := g.tokens[token]
		if !ok {
			// handles both expiry _and_ caller gave us an unknown token
			continue
		}
		td.sessions[gs] = struct{}{}
		anyValid = true

		// record use
		gs.tokens[token] = struct{}{}
	}

	// inform that there's new tokens (in a non-blocking way)
	if !anyValid {
		gs.Stop()
	} else if !gs.tokenUpdateChClosed {
		select {
		case gs.tokenUpdateCh <- struct{}{}:
		default:
		}
	}

	g.tokenLock.Unlock()
}

func (g *guardImpl[Token, Key]) RunSession(ctx context.Context, key Key) (Session[Token], error) {
	// take slice that we can use (CheckSession might take a while)
	g.tokenLock.RLock()
	tokens := make([]Token, 0, len(g.tokens))
	for t := range g.tokens {
		tokens = append(tokens, t)
	}
	g.tokenLock.RUnlock()

	use, err := g.check(ctx, key, tokens) // always called even with zero tokens
	if err != nil {
		return nil, err
	}

	derivedCtx, cancel := context.WithCancelCause(ctx)

	tokenUpdateCh := make(chan struct{}, 1)
	gs := &guardSession[Token, Key]{
		key:           key,
		derivedCtx:    derivedCtx,
		cancel:        cancel,
		tokens:        map[Token]struct{}{},
		tokenUpdateCh: tokenUpdateCh,
	}

	g.installSession(gs, use)

	return gs, nil
}

func New[Token comparable, Key any](check CheckFunc[Token, Key]) Guard[Token, Key] {
	return &guardImpl[Token, Key]{
		check:  check,
		tokens: map[Token]*tokenData[Token, Key]{},
	}
}

type guardSession[Token comparable, Key any] struct {
	key        Key
	derivedCtx context.Context
	cancel     context.CancelCauseFunc

	tokenLock           sync.RWMutex
	tokens              map[Token]struct{}
	tokenUpdateCh       chan struct{} // simply fired when new tokens are available
	tokenUpdateChClosed bool
}

func (gs *guardSession[Token, Key]) Context() context.Context {
	return gs.derivedCtx
}

func (gs *guardSession[Token, Key]) UpdateCh() <-chan struct{} {
	return gs.tokenUpdateCh
}

func (gs *guardSession[Token, Key]) Stop() {
	// TODO: if a token never expires, the session will never be truly GC'ed
	gs.cancel(nil)

	gs.tokenLock.Lock()
	defer gs.tokenLock.Unlock()

	if !gs.tokenUpdateChClosed {
		close(gs.tokenUpdateCh)
		gs.tokenUpdateChClosed = true
	}
}

func (gs *guardSession[Token, Key]) Tokens() []Token {
	gs.tokenLock.RLock()
	defer gs.tokenLock.RUnlock()

	out := make([]Token, 0, len(gs.tokens))
	for k := range gs.tokens {
		out = append(out, k)
	}
	return out
}
