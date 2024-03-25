package main

import (
	"errors"
	"fmt"
)

func main() {
	builtinFunc()
	staticFunc()
	staticFuncClosure()
	anonymousStaticFunc()
	staticMethod(Foo{name: "foo"})
	dynamicMethod(Foo{name: "foo"})
	builtinDynamicMethod(errors.New("error"))
	dynamicFuncCall(func() string { return "foo" })
	dynamicFuncCall2()
	dynamicFuncCall3()
	genericsStaticFunc()
}

func builtinFunc() {
	s := append([]int{1, 2, 3}, 4)
	fmt.Println(s)
}

func staticFunc() {
	fmt.Println("staticFunc")
}

func staticFuncClosure() {
	count := 0
	func() int {
		count++
		return count
	}()
}

func anonymousStaticFunc() {
	func() {
		fmt.Println("staticFuncClosure")
	}()
}

func dynamicFuncCall(fn func() string) {
	fmt.Println(fn())
}

var callableVar = func() string { return "foo" }

func dynamicFuncCall2() {
	fmt.Println(callableVar())
}

func dynamicFuncCall3() {
	c := getCallable()
	fmt.Println(c())
}

func getCallable() func() int {
	return func() int {
		return 0
	}
}

func genericsStaticFunc() {
	fmt.Println(foo[Stringer](Stringer{name: "foo"}))
}

type Stringer struct {
	name string
}

func (s Stringer) String() string {
	return s.name
}

func foo[T fmt.Stringer](t T) string {
	return t.String()
}

type Foo struct {
	name string
}

func (f Foo) String() string {
	return f.name
}

func staticMethod(f Foo) {
	name := f.String()
	fmt.Println(name)
}

func dynamicMethod(f fmt.Stringer) {
	name := f.String()
	fmt.Println(name)
}

func builtinDynamicMethod(err error) {
	msg := err.Error()
	fmt.Println(msg)
}
