package weave

import "log"

const Main byte = 1
const Background byte = 2

type chainedFunc[T any] struct {
	thread byte
	fn     func(*T)
}

type status struct {
	thread  byte
	pending bool
}

type ChainedTask[T any] struct {
	ctx *T
	fns []chainedFunc[T]
	idx int
	s   status
}

func NewChain[T any](ctx *T, cap int) *ChainedTask[T] {
	return &ChainedTask[T]{
		ctx: ctx,
		fns: make([]chainedFunc[T], 0, cap),
		idx: 0,
		s:   status{Main, true},
	}
}

func (t *ChainedTask[T]) Add(thread byte, fn func(*T)) *ChainedTask[T] {
	if t.idx > 0 {
		log.Fatal("Can't chain after dispatched!")
	}
	t.fns = append(t.fns, chainedFunc[T]{thread: thread, fn: fn})
	return t
}

func (t *ChainedTask[T]) Status() (byte, bool) {
	return t.s.thread, t.s.pending
}

func (t *ChainedTask[T]) SetStatus(thread byte, pending bool) {
	t.s.thread = thread
	t.s.pending = pending
}

func (t *ChainedTask[T]) CurrentThread() byte {
	return t.fns[t.idx].thread
}

func (t *ChainedTask[T]) Execute() bool {
	if t.idx < len(t.fns) {
		t.fns[t.idx].fn(t.ctx)
		t.idx += 1
		return true
	}
	return false
}

type Task interface {
	Status() (byte, bool)
	SetStatus(byte, bool)
	CurrentThread() byte
	Execute() bool
}

type taskReport struct {
	idx int
	ok  bool
}
type WorkerPool struct {
	tasks    []Task
	dispatch chan int
	resolve  chan taskReport
}

func NewWorkerPool(cap int) *WorkerPool {
	w := &WorkerPool{
		tasks:    make([]Task, 1024),
		dispatch: make(chan int, 1024),
		resolve:  make(chan taskReport, 1024),
	}

	for i := 0; i < cap; i++ {
		go func() {
			for i := range w.dispatch {
				w.resolve <- taskReport{idx: i, ok: w.tasks[i].Execute()}
			}
		}()
	}

	return w
}

func (w *WorkerPool) Dispatch(t Task) bool {
	idx := -1
	for i := 0; i < len(w.tasks); i++ {
		if w.tasks[i] == nil {
			idx = i
			break
		}
	}

	if idx == -1 {
		return false
	}

	w.tasks[idx] = t
	return true
}

func (w *WorkerPool) Dispose() {
	close(w.dispatch)
	close(w.resolve)
}

func (w *WorkerPool) Poll() {
	for i := range w.tasks {
		if w.tasks[i] == nil {
			continue
		}

		if t, p := w.tasks[i].Status(); p {
			if t == Main {
				if !w.tasks[i].Execute() {
					w.tasks[i] = nil
				}
			} else {
				w.tasks[i].SetStatus(Background, false)
				w.dispatch <- i
			}
		}
	}

	select {
	case r := <-w.resolve:
		if r.ok {
			w.tasks[r.idx].SetStatus(Main, true)
		} else {
			w.tasks[r.idx] = nil
		}
	default:
	}
}
