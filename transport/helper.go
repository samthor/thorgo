package transport

// ReadJSONCh creates a read channel for this [Transport].
// It is closed on the first read error.
func ReadJSONCh[X any](t Transport) <-chan X {
	ch := make(chan X)

	go func() {
		for {
			var x X

			err := t.ReadJSON(&x)
			if err != nil {
				close(ch)
				return
			}

			ch <- x
		}
	}()

	return ch
}

// WriteJSONCh creates a write channel for this [Transport].
// It is the caller's requirement to close the channel.
func WriteJSONCh[X any](t Transport) chan<- X {
	ch := make(chan X)

	go func() {
		for x := range ch {
			t.WriteJSON(x)
		}
	}()

	return ch
}
