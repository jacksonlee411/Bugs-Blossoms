package eventbus

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/iota-uz/iota-sdk/pkg/serrors"
	"github.com/sirupsen/logrus"
)

type Subscriber struct {
	Handler interface{}
}

type EventBus interface {
	Publish(args ...interface{})
	Subscribe(handler interface{})
	Unsubscribe(handler interface{})
	Clear()
	SubscribersCount() int
}

type EventBusWithError interface {
	EventBus
	PublishE(args ...any) error
}

var (
	ErrNoSubscribers        = serrors.NewError("EVENTBUS_NO_SUBSCRIBERS", "no matching subscribers", "")
	ErrInvalidHandlerReturn = serrors.NewError("EVENTBUS_INVALID_HANDLER_RETURN", "invalid handler return signature", "")
)

type publisherImpl struct {
	log         *logrus.Logger
	Subscribers []Subscriber
}

func NewEventPublisher(log *logrus.Logger) EventBus {
	return &publisherImpl{log: log}
}

func MatchSignature(handler interface{}, args []interface{}) bool {
	t := reflect.TypeOf(handler)
	if t.Kind() != reflect.Func {
		return false
	}

	if t.NumIn() != len(args) {
		return false
	}

	for i, arg := range args {
		paramType := t.In(i)
		argType := reflect.TypeOf(arg)

		// Handle nil arguments
		if arg == nil {
			if paramType.Kind() != reflect.Interface && paramType.Kind() != reflect.Ptr {
				return false
			}
			continue
		}

		// If the parameter is an interface, check if argument implements it
		if paramType.Kind() == reflect.Interface {
			if !argType.Implements(paramType) {
				return false
			}
			continue
		}

		// For other types, check direct assignability
		if !argType.AssignableTo(paramType) {
			return false
		}
	}

	return true
}

func (p *publisherImpl) Publish(args ...interface{}) {
	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		in[i] = reflect.ValueOf(arg)
	}

	handled := false
	for _, subscriber := range p.Subscribers {
		v := reflect.ValueOf(subscriber.Handler)
		if !MatchSignature(subscriber.Handler, args) {
			continue
		}
		// Wrap handler invocation with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					handlerName := v.Type().String()
					// Log panic with error level and include event args for debugging
					if p.log != nil {
						p.log.Errorf("eventbus: handler %s panicked with args %v: %v", handlerName, args, r)
					}
				}
			}()
			v.Call(in)
			// Only mark as handled if handler completed successfully without panic
			handled = true
		}()
	}

	if !handled {
		if p.log != nil {
			p.log.Warnf("eventbus.Publish: no matching subscribers for event with args: %v", in)
		}
		return
	}
}

func (p *publisherImpl) PublishE(args ...any) error {
	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		in[i] = reflect.ValueOf(arg)
	}

	handled := false
	var errs []error

	for _, subscriber := range p.Subscribers {
		v := reflect.ValueOf(subscriber.Handler)
		if !MatchSignature(subscriber.Handler, args) {
			continue
		}

		handled = true

		func() {
			defer func() {
				if r := recover(); r != nil {
					handlerName := v.Type().String()
					panicErr := fmt.Errorf("eventbus: handler %s panicked: %v", handlerName, r)
					errs = append(errs, panicErr)
				}
			}()

			out := v.Call(in)
			if len(out) == 0 {
				return
			}
			if len(out) != 1 {
				errs = append(errs, fmt.Errorf("%w: handler %s returned %d values", ErrInvalidHandlerReturn, v.Type().String(), len(out)))
				return
			}

			ret := out[0]
			if ret.Type() != reflect.TypeOf((*error)(nil)).Elem() {
				errs = append(errs, fmt.Errorf("%w: handler %s return type is %s", ErrInvalidHandlerReturn, v.Type().String(), ret.Type().String()))
				return
			}
			if !ret.IsNil() {
				errs = append(errs, ret.Interface().(error))
			}
		}()
	}

	if !handled {
		return ErrNoSubscribers
	}
	return errors.Join(errs...)
}

func (p *publisherImpl) Subscribe(handler interface{}) {
	t := reflect.TypeOf(handler)
	if t.Kind() != reflect.Func {
		panic("handler must be a function")
	}
	p.Subscribers = append(
		p.Subscribers,
		Subscriber{Handler: handler},
	)
}

func (p *publisherImpl) Unsubscribe(handler interface{}) {
	for i, subscriber := range p.Subscribers {
		if subscriber.Handler == handler {
			p.Subscribers = append(p.Subscribers[:i], p.Subscribers[i+1:]...)
			return
		}
	}
}

func (p *publisherImpl) Clear() {
	p.Subscribers = []Subscriber{}
}

func (p *publisherImpl) SubscribersCount() int {
	return len(p.Subscribers)
}
