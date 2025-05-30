package utils

type Queue[T any] struct {
	slice []T
	Size  int
}

func (q *Queue[T]) Push(val T) {
	q.slice = append(q.slice, val)
	q.Size++
}

func (q *Queue[T]) Pop() (T, bool) {
	if len(q.slice) == 0 {
		var none T
		return none, false
	}

	val := q.slice[0]
	q.slice = q.slice[1:]

	q.Size--

	return val, true
}
