package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	//RawCall(os.Stdout,"","Register","1","13210600685","蔡徐坤","hehe","0")
	//RawCall(os.Stdout,"11111","Puton","66","5","asd","6")

	// Step 1
	ret := CallRet("", "Login", "1", "132110600685", "qwer")
	token := strings.Split(ret.Rets[0], "\x1f")[0]

	log.Print("token:", token)
	// Step 2
	testcall := func(method string, args ...string) {
		RawCall(os.Stdout, token, method, args...)
	}

	testcall("Search", "")
	testcall("Charge", "0")
}

func test_of_pressure() {
	start := time.Now()
	wg := &sync.WaitGroup{}

	ch := make(chan func(), 1000)

	for i := 0; i < 10; i++ {
		go func() {
			for fn := range ch {
				fn()
			}
		}()
	}

	for i := 0; i < 10000; i++ {
		wg.Add(1)
		ch <- func() {
			defer wg.Done()
			call()
		}
	}

	wg.Wait()

	fmt.Println(time.Now().Sub(start))
}

func CallRet(token string, method string, args ...string) Ret {
	buf := bytes.NewBuffer(nil)
	RawCall(buf, token, method, args...)
	var ret Ret
	json.Unmarshal(buf.Bytes(), &ret)
	return ret
}

func RawCall(w io.Writer, token, method string, args ...string) {
	buf := bytes.NewBuffer(nil)
	json.NewEncoder(buf).Encode(map[string]interface{}{
		"token":  token,
		"method": method,
		"args":   []string{strings.Join(args, "\x1f")},
	})
	resp, err := http.Post("http://localhost:8081/api", "",
		buf)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	io.Copy(w, resp.Body)
}

func call() {
	RawCall(os.Stdout, "", "Login", "1", "13210600685", "qwer")
}

type Req struct {
	Method string   `json:"method"` //识别
	Token  string   `json:"token"`
	Args   []string `json:"args"'` //字符串
}
type Ret struct {
	Status string   `json:"status"` // ok | err
	Rets   []string `json:"rets"`
}

func Call(req Req) Ret {
	data, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post("http://localhost:8081/", "",
		strings.NewReader(string(data)))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var ret Ret
	err = json.NewDecoder(resp.Body).Decode(&ret)
	if err != nil {
		panic(err)
	}
	return ret
}
