package integrity

import "errors"

// ValidationError returns the validate failure inside an ErrExhausted chain, or nil
// when err is nil or not exhausted.
func ValidationError(err error) error {
	if err == nil || !errors.Is(err, ErrExhausted) {
		return nil
	}
	return findNonSentinelCause(err)
}

func findNonSentinelCause(err error) error {
	type multiUnwrap interface {
		Unwrap() []error
	}
	seen := map[error]struct{}{}
	stack := []error{err}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == nil {
			continue
		}
		if _, ok := seen[cur]; ok {
			continue
		}
		seen[cur] = struct{}{}

		if mu, ok := cur.(multiUnwrap); ok {
			for _, sub := range mu.Unwrap() {
				if sub != nil && !errors.Is(sub, ErrExhausted) && !errors.Is(sub, ErrInvalid) {
					return sub
				}
				stack = append(stack, sub)
			}
			continue
		}
		var ie *Error
		if errors.As(cur, &ie) && ie.err != nil {
			stack = append(stack, ie.err)
			continue
		}
		if u := errors.Unwrap(cur); u != nil {
			if !errors.Is(u, ErrExhausted) && !errors.Is(u, ErrInvalid) {
				var inner *Error
				if errors.As(u, &inner) {
					stack = append(stack, u)
					continue
				}
				return u
			}
			stack = append(stack, u)
		}
	}
	return nil
}
