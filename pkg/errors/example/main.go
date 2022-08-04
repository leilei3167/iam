package main

import (
	"fmt"

	"github.com/marmotedu/iam/pkg/errors"
)

func main() {
	e := err3()
	fmt.Println("got err:", e)
	fmt.Println()
	fmt.Printf("%+v", e)
}

func err1() error {
	e := errors.New("first error!!!")
	// fmt.Printf("first err is: %v\n", e)
	// fmt.Printf("first err with stack trace: %+v", e)
	return e
}

func err2() error {
	e := err1()
	return errors.Wrap(e, "something")
}

func err3() error {
	e := err2()
	return errors.WrapC(e, 123321, "this is wrapC")
}
