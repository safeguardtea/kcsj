package main

import (
	"fmt"
	"strings"
)

func main() {
	str := "我槽哈哈"
	str1 := strings.Split(str, "")
	str2 := strings.Join(str1, "%")
	str2 = "'%" + str2 + "%'"

	fmt.Println(str2)
}
