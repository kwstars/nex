package nex

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
)

// HandlerAdapter represents a container that contain a handler function
// and convert a it to a http.Handler
type HandlerAdapter interface {
	Invoke(context.Context, http.ResponseWriter, *http.Request) (context.Context, interface{}, error)
}

// genericAdapter represents a common adapter
type genericAdapter struct {
	inContext  bool
	outContext bool
	method     reflect.Value
	numIn      int
	types      []reflect.Type
	cacheArgs  []reflect.Value // cache args
}

// Accept zero parameter adapter
type simplePlainAdapter struct {
	inContext  bool
	outContext bool
	method     reflect.Value
	cacheArgs  []reflect.Value
}

// Accept only one parameter adapter
type simpleUnaryAdapter struct {
	outContext bool
	argType    reflect.Type
	method     reflect.Value
	cacheArgs  []reflect.Value // cache args
}

func makeGenericAdapter(method reflect.Value, inContext, outContext bool) *genericAdapter {
	noSupportExists := false
	t := method.Type()
	numIn := t.NumIn()

	a := &genericAdapter{
		inContext:  inContext,
		outContext: outContext,
		method:     method,
		numIn:      numIn,
		types:      make([]reflect.Type, numIn),
		cacheArgs:  make([]reflect.Value, numIn),
	}

	for i := 0; i < numIn; i++ {
		in := t.In(i)
		if in != contextType && !isSupportType(in) {
			if noSupportExists {
				panic("function should accept only one customize type")
			}

			if in.Kind() != reflect.Ptr {
				panic("customize type should be a pointer(" + in.PkgPath() + "." + in.Name() + ")")
			}
			noSupportExists = true
		}
		a.types[i] = in
	}

	return a
}

func (a *genericAdapter) Invoke(ctx context.Context, w http.ResponseWriter, r *http.Request) (
	outCtx context.Context, payload interface{}, err error,
) {
	outCtx = ctx
	// Create a new slice for each invocation to avoid race conditions
	values := make([]reflect.Value, a.numIn)
	for i := 0; i < a.numIn; i++ {
		typ := a.types[i]
		v, ok := supportTypes[typ]
		if ok {
			values[i] = v(r)
		} else if typ == contextType {
			values[i] = reflect.ValueOf(ctx)
		} else {
			d := reflect.New(a.types[i].Elem()).Interface()
			if err = json.NewDecoder(r.Body).Decode(d); err != nil {
				panic(err)
			}
			values[i] = reflect.ValueOf(d)
		}
	}

	ret := a.method.Call(values)

	if a.outContext {
		outCtx = ret[0].Interface().(context.Context)
		payload = ret[1].Interface()
		if e := ret[2].Interface(); e != nil {
			err = e.(error)
		}
	} else {
		payload = ret[0].Interface()
		if e := ret[1].Interface(); e != nil {
			err = e.(error)
		}
	}

	return
}

func (a *simplePlainAdapter) Invoke(ctx context.Context, w http.ResponseWriter, r *http.Request) (
	outCtx context.Context, payload interface{}, err error,
) {
	outCtx = ctx

	var values []reflect.Value
	if a.inContext {
		values = []reflect.Value{reflect.ValueOf(ctx)}
	} else {
		values = []reflect.Value{}
	}

	// call it
	ret := a.method.Call(values)

	if a.outContext {
		outCtx = ret[0].Interface().(context.Context)
		payload = ret[1].Interface()
		if e := ret[2].Interface(); e != nil {
			err = e.(error)
		}
	} else {
		payload = ret[0].Interface()
		if e := ret[1].Interface(); e != nil {
			err = e.(error)
		}
	}

	return
}

func (a *simpleUnaryAdapter) Invoke(ctx context.Context, w http.ResponseWriter, r *http.Request) (
	outCtx context.Context, payload interface{}, err error,
) {
	outCtx = ctx
	data := reflect.New(a.argType.Elem()).Interface()
	if err = json.NewDecoder(r.Body).Decode(data); err != nil {
		panic(err)
	}

	values := []reflect.Value{reflect.ValueOf(data)}
	ret := a.method.Call(values)

	if a.outContext {
		outCtx = ret[0].Interface().(context.Context)
		payload = ret[1].Interface()
		if e := ret[2].Interface(); e != nil {
			err = e.(error)
		}
	} else {
		payload = ret[0].Interface()
		if e := ret[1].Interface(); e != nil {
			err = e.(error)
		}
	}

	return
}
