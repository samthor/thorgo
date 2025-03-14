package transport

// ReadCh accepts the given transport and takes over its read handler to send messages on the passed channel.
// This continues until the Transport shuts down.
func ReadCh[Out any](tr Transport) <-chan Out {
	ch := make(chan Out)

	go func() {
		for {
			var out Out

			err := tr.Read(&out)
			if err != nil {
				close(ch)
				return
			}

			ch <- out
		}
	}()

	return ch
}
