package seo

import (
	"fmt"
	"github.com/qor5/admin/seo/user1"
	"github.com/qor5/admin/seo/user2"
	"reflect"
	"testing"
)

func TestA(t *testing.T) {
	u1 := user1.User{}
	u2 := user2.User{}
	fmt.Println(reflect.TypeOf(u1).Name())
	fmt.Println(reflect.TypeOf(u2).Name())
}
