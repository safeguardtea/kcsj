package main

import (
	"fmt"
	"time"
)

func num_sleep(sec int, c chan int) {
	time.Sleep(100 * time.Millisecond * time.Duration(sec))
	c <- sec
}
func main() {
	arr := []int{6, 5, 7, 8, 4, 0, 1, 2, 9, 3} //长度为10
	c := make(chan int, 10)
	for _, num := range arr {
		go num_sleep(num, c)
	}
	var res []int
	for i := 0; i < 10; i++ {
		res = append(res, <-c)
	}
	close(c)
	fmt.Println(res)
}
