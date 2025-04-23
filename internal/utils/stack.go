package utils

type Stack[T any] struct {
	slice []T
	Size  int
}

func (q *Stack[T]) Push(val T) {
	q.slice = append(q.slice, val)
	q.Size++
}

func (q *Stack[T]) Pop() (T, bool) {
	if len(q.slice) == 0 {
		var none T
		return none, false
	}

	val := q.slice[len(q.slice)-1]
	q.slice = q.slice[:len(q.slice)-1]

	q.Size--

	return val, true
}
