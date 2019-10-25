package main

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const indexHTML = `
<!DOCTYPE html>
<html>
<head>
	<title>演示</title>
</head>
<body>
ok
<script>
	// Example POST method implementation:

function call(method,args,fnok,fnerr) {
	postData('http://localhost:8081/api', {method:method,args:args.join("\x1f")})
 	.then(ret => {
	if (ret.status == "ok"){
		fnok(ret.rets)
	}else if (ret.status == "err") {
		fnerr(ret.rets[0])
	}
	})
 	.catch(error => console.error(error))
}

function calldef(method,args) {
call(method,args,(rets)=>{
console.log(rets.join(" "));
},err=>{
console.error(err);
})
}

call("echo",["end"],(rets)=>{
console.log(rets.join(" "));
},err=>{
console.error(err);
})

function postData(url, data) {
 // Default options are marked with *
 return fetch(url, {
   body: JSON.stringify(data), // must match 'Content-Type' header
   cache: 'no-cache', // *default, no-cache, reload, force-cache, only-if-cached
   credentials: 'same-origin', // include, same-origin, *omit
   headers: {
     'user-agent': 'Mozilla/4.0 MDN Example',
     'content-type': 'application/json'
   },
   method: 'POST', // *GET, POST, PUT, DELETE, etc.
   mode: 'cors', // no-cors, cors, *same-origin
   redirect: 'follow', // manual, *follow, error
   referrer: 'no-referrer', // *client, no-referrer
 })
 .then(response => response.json()) // parses response to JSON
}
</script>
</body>
</html>
`

type Req struct {
	Uid    int
	Method string   `json:"method"` //识别
	Token  string   `json:"token"`
	Args   []string `json:"args"'` //字符串
}
type Ret struct {
	Status string   `json:"status"` // ok | err | auth
	Rets   []string `json:"rets"`
}

func WriteError(w http.ResponseWriter, err error) {
	ret := Ret{
		Status: "err",
		Rets:   []string{err.Error()},
	}

	json.NewEncoder(w).Encode(&ret) //写入web
}

func Request(id int, method, token string, args ...string) Req {
	var temp Req
	temp.Uid = id
	temp.Method = method
	temp.Token = token
	temp.Args = make([]string, len(args))
	for num, single := range args {
		temp.Args[num] = single
	}
	return temp
}

type ErrorWithMessage struct {
	Msg string
}

func (e *ErrorWithMessage) Error() string {
	return e.Msg
}

func GetMessageFromError(v interface{}) string {
	if e, ok := v.(*ErrorWithMessage); ok {
		return e.Msg
	}
	return "服务器内部错误"
}

func RetErrorWithMessage(msg string) {
	panic(&ErrorWithMessage{Msg: msg})
}

type HandleFunc func(Req) Ret
type AuthFunc func(*Req) bool

//Echo函数
func Echo(req Req) Ret {
	return Ret{
		Status: "ok",
		Rets:   req.Args,
	}
}

var Db *sqlx.DB           //数据库
var Tokens map[string]int //tokens
var mutex_tokens = new(sync.Mutex)
var mutex_purchase = new(sync.Mutex)

func getToken(id int) string { ///得到token值
	mutex_tokens.Lock()
	defer mutex_tokens.Unlock()

	t := strconv.FormatInt(time.Now().Unix(), 10)
	h := md5.New()
	io.WriteString(h, t)
	//fmt.Printf("%x", h.Sum(nil))
	token := fmt.Sprintf("%x", h.Sum(nil))
	Tokens[token] = id
	return token
}

func verifyToken(token string) int {
	mutex_tokens.Lock()
	defer mutex_tokens.Unlock()
	id, exist := Tokens[token]
	if exist != true {
		return -1
	} else {
		return id
	}
}

func connectDb() { //连接数据库
	database, err := sqlx.Open("mysql", "root:123456789@tcp(127.0.0.1:3306)/firstdb")
	if err != nil {
		fmt.Println("Open mysql failed", err)
		return
	}
	Db = database
}

//检查账号是否重复
type BUYERS struct {
	Id       int    `db:"id"`
	Name     string `db:"name"`
	Account  int64  `db:"account"`
	Password string `db:"password"`
	Money    int    `db:"money"`
}
type SELLERS struct {
	Id          int    `db:"id"`
	Name        string `db:"name"`
	Account     int64  `db:"account"`
	Password    string `db:"password"`
	Adv_payment int    `db:"adv_payment"`
	Money       int    `db:"money"`
}

func checkUserForLogin(n int, goAccount int64, password string) (uid int, name string) {
	if n == 0 {
		var seller []SELLERS
		err := Db.Select(&seller, "select id,name from sellers where account=? and password=?", goAccount, password)
		if err != nil {
			panic(err)
		}
		if len(seller) == 0 {
			RetErrorWithMessage("账号或密码错误")
		}
		return seller[0].Id, seller[0].Name
	}

	if n == 1 {
		var buyer []BUYERS
		err := Db.Select(&buyer, "select id,name from buyers where account=? and password=?", goAccount, password)
		if err != nil {
			panic(err)
		}
		if len(buyer) == 0 {
			RetErrorWithMessage("账号或密码错误")
		}
		return buyer[0].Id, buyer[0].Name
	}
	panic("TODO")
}

func Login(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	Id, _ := strconv.Atoi(temp[0])
	account, _ := strconv.ParseInt(temp[1], 10, 64)
	password := temp[2]
	uid, name := checkUserForLogin(Id, account, password)
	log.Println("TRY:Login:", account, password, "GOT:", uid, name)
	token := getToken(uid)
	return Ret{
		Status: "ok",
		Rets:   []string{token + "\x1f" + name},
	}
}

func checkUserRepeat(goAccount int64, name string) bool {
	var seller1, seller2 []SELLERS
	Db.Select(&seller1, "select id,name from sellers where account=?", goAccount)
	if len(seller1) != 0 {
		RetErrorWithMessage("账号重复")
	}
	Db.Select(&seller2, "select id,name from sellers where name=?", name)
	if len(seller2) != 0 {
		RetErrorWithMessage("昵称重复")
	}
	var buyer1, buyer2 []BUYERS
	Db.Select(&buyer1, "select id,name from buyers where account=?", goAccount)
	if len(buyer1) != 0 {
		RetErrorWithMessage("账号重复")
	}
	Db.Select(&buyer2, "select id,name from buyers where name=?", name)
	if len(buyer2) != 0 {
		RetErrorWithMessage("昵称重复")
	}
	return true
}

func Register(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	log.Println(temp)
	Id, err := strconv.Atoi(temp[0])
	if err != nil {
		panic(err)
	}
	account, err := strconv.ParseInt(temp[1], 10, 64)
	if err != nil {
		RetErrorWithMessage("账号必须为手机号")
	}
	password := temp[2]
	name := temp[3]
	if checkUserRepeat(account, name) {
		if Id == 0 {
			_, err = Db.Exec("insert into sellers(account,password,name,money,adv_payment) values (?,?,?,0,0)", account, password, name)
			if err != nil {
				panic(err)
			}
			log.Println("insert succeed")
		}
		if Id == 1 {
			_, err = Db.Exec("insert into buyers(account,password,name,money) values (?,?,?,0)", account, password, name)
			if err != nil {
				panic(err)
			}
			log.Println("insert succeed")
		}
		return Ret{
			Status: "ok",
			Rets:   []string{},
		}
	}
	return Ret{}
}

type GOODS struct {
	Id        int    `db:"id"`
	Name      string `db:"name"`
	Image     string `db:"image"`
	Price     int    `db:"price"`
	Introduce string `db:"introduce"`
	Others    string `db:"others"`
	Number    int    `db:"amount"`
	Seller_id int    `db:"seller_id"`
}

func Puton(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	name := temp[0]
	price, _ := strconv.Atoi(temp[1])
	introduce := temp[2]
	number, _ := strconv.Atoi(temp[3])
	imageData := temp[4]
	seller_id := verifyToken(req.Token)
	_, err := Db.Exec("insert into goods(name, image, price, introduce, others, amount, seller_id) values (?,?,?,?,'',?,?)", name, imageData, price, introduce, number, seller_id)
	if err != nil {
		//RetErrorWithMessage("服务器错误")
		panic(err)
	}
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

type COUPON_TYPE struct {
	Id          int    `db:"id"`
	Seller_id   int    `db:"seller_id"`
	Good_id     int    `db:"good_id"`
	Amount      int    `db:"amount"`
	Others      string `db:"others"`
	Deadline    string `db:"deadline"`
	IsAvailable int    `db:"isAvailable"`
}

func Setpreferential(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	Id, _ := strconv.Atoi(temp[0])
	others := temp[1]
	var good []GOODS
	err := Db.Select(&good, "select * from goods where id =?", Id)
	if err != nil {
		panic(err)
	}
	if len(good) == 0 {
		RetErrorWithMessage("查无此商品")
	}
	Db.Exec("update goods set others=? where id =?", others, Id)
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

func Setcoupon(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	Id, _ := strconv.Atoi(temp[0])
	number, _ := strconv.Atoi(temp[1])
	others := temp[2]
	timestamp := temp[3]
	var good []GOODS
	err := Db.Select(&good, "select id from goods where id =?", Id)
	if err != nil {
		panic(err)
	}
	if len(good) == 0 {
		RetErrorWithMessage("查无此商品")
	}
	seller_id := verifyToken(req.Token)
	Db.Exec("insert into coupons_type(seller_id, good_id, amount, others, deadline,isAvailable) values (?,?,?,?,?,1)", seller_id, Id, number, others, timestamp)
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

type COUPONS struct {
	ID        int `db:"id"`
	Seller_id int `db:"seller_id"`
	Buyers_id int `db:"buyers_id"`
	Coupon_id int `db:"coupon_id"`
}

func Getcoupon(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	Id, _ := strconv.Atoi(temp[0])
	var coupon_type []COUPON_TYPE
	err := Db.Select(&coupon_type, "select * from coupons_type where id=?", Id)
	if err != nil {
		panic(err)
	}
	if len(coupon_type) == 0 {
		RetErrorWithMessage("查无此优惠券")
	}
	if coupon_type[0].IsAvailable == 0 {
		RetErrorWithMessage("该优惠券已失效")
	}
	uid := verifyToken(req.Token)
	seller_id := coupon_type[0].Seller_id
	var coupons []COUPONS
	err1 := Db.Select(&coupons, "select * from coupons where buyers_id=?", uid)
	if err1 != nil {
		panic(err1)
	}
	flag := false
	if len(coupons) != 0 {
		for _, single := range coupons {
			if single.Coupon_id == Id {
				flag = true
			}
		}
	}
	if flag {
		RetErrorWithMessage("已经领了该优惠券")
	}
	_, err = Db.Exec("insert into coupons(seller_id, buyers_id, coupon_id) values (?,?,?)", seller_id, uid, Id)
	if err != nil {
		panic(err)
	}
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

type CART struct {
	Id       int `db:"id"`
	Good_id  int `db:"good_id"`
	Number   int `db:"number"`
	Buyer_id int `db:"buyer_id"`
}

func Putcart(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	good_id, _ := strconv.Atoi(temp[0])
	var good []GOODS
	Db.Select(&good, "select * from goods where id=?", good_id)
	if len(good) == 0 {
		RetErrorWithMessage("无此商品")
	}
	buyer_id := verifyToken(req.Token)
	var cart []CART
	Db.Select(&cart, "select * from carts where buyer_id=?", buyer_id)
	flag := false
	if len(cart) != 0 {
		for _, single := range cart {
			if single.Good_id == good_id {
				flag = true
			}
		}
	}
	if flag {
		RetErrorWithMessage("已经在购物车中")
	}
	Db.Exec("insert into carts(good_id,buyer_id) values (?,?)", good_id, buyer_id)
	return Ret{
		Status: "ok",
		Rets:   []string{"ok"},
	}
}

func Biringcart(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	good_id, _ := strconv.Atoi(temp[0])
	buyer_id := verifyToken(req.Token)
	var cart []CART
	Db.Select(&cart, "select * from carts where buyer_id=?", buyer_id)
	flag := false
	if len(cart) != 0 {
		for _, single := range cart {
			if single.Good_id == good_id {
				flag = true
			}
		}
	}
	if flag == false {
		RetErrorWithMessage("购物车内查无此商品")
	}
	Db.Exec("delete from carts where(good_id=? and buyer_id=?)", good_id, buyer_id)
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

func Scancart(req Req) Ret {
	var cart []CART
	buyer_id := verifyToken(req.Token)
	err := Db.Select(&cart, "select * from carts where buyer_id= ?", buyer_id)
	if err != nil {
		panic(err)
	}
	tocode := make([]string, len(cart), len(cart)+5)
	for i := 0; i < len(cart); i++ {
		var good []GOODS
		err = Db.Select(&good, "select * from goods where id=?", cart[i].Good_id)
		if err != nil {
			panic(err)
		}
		tocode[i] = strconv.Itoa(good[0].Id) + "\x1f" + good[0].Name + "\x1f" + strconv.Itoa(good[0].Price) + "\x1f" + strconv.Itoa(good[0].Number) + "\x1f" + strconv.Itoa(cart[i].Id) + "\x1f" + good[0].Others
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

type TRADES struct {
	ID              int    `db:"id"`
	Seller_id       int    `db:"seller_id"`
	Buyer_id        int    `db:"buyer_id"`
	Good_id         int    `db:"good_id"`
	Price           int    `db:"price"`
	Number          int    `db:"number"`
	Status          string `db:"status"`
	Time            string `db:"time"`
	UUID            string `db:"UUID"`
	Transport_order string `db:"transport_order"`
}

//参数的含义0/1分别表示卖家/买家；表示身份;0/1分别表示钱数增加减少；表示钱数（以分来计算）
func changemoney(person, id, instruction, number int) {
	if person == 0 {
		if instruction == 0 {
			_, err := Db.Exec("update sellers set money=money+? where id =?", number, id)
			if err != nil {
				RetErrorWithMessage("钱数增加错误")
			}
		}
		if instruction == 1 {
			var seller []SELLERS
			Db.Select(&seller, "select *from sellers where id=?", id)
			if len(seller) == 0 {
				RetErrorWithMessage("没有该用户")
			}
			if seller[0].Money-number < 0 {
				RetErrorWithMessage("余额不足")
			}
			Db.Exec("update sellers set money=money-? where id=?", number, id)
		}
	}
	if person == 1 {
		if instruction == 0 {
			_, err := Db.Exec("update buyers set money=money+? where id=?", number, id)
			if err != nil {
				RetErrorWithMessage("钱数增加错误")
			}
		}
		if instruction == 1 {
			var buyer []BUYERS
			Db.Select(&buyer, "select *from buyers where id=?", id)
			if len(buyer) == 0 {
				RetErrorWithMessage("没有该用户")
			}
			if buyer[0].Money-number < 0 {
				RetErrorWithMessage("余额不足")
			}
			Db.Exec("update buyers set money=money-? where id=?", number, id)
		}
	}
}

func Charge(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	number, err := strconv.Atoi(temp[0])
	if err != nil {
		panic(err)
	}
	buyer_id := verifyToken(req.Token)
	_, err = Db.Exec("update buyers set money=money+? where id=?", number, buyer_id)
	if err != nil {
		panic(err)
	}
	var buyer []BUYERS
	err = Db.Select(&buyer, "select * from buyers where id =?", buyer_id)
	if err != nil {
		panic(err)
	}
	if len(buyer) == 0 {
		RetErrorWithMessage("无该用户")
	}
	balance := strconv.Itoa(buyer[0].Money)
	return Ret{
		Status: "ok",
		Rets:   []string{balance},
	}
}

func Scancharge(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	NUM := temp[0]
	user_id := verifyToken(req.Token)
	if NUM == "0" {
		var seller []SELLERS
		err := Db.Select(&seller, "select * from sellers where id= ?;", user_id)
		if err != nil {
			panic(err)
		}
		if len(seller) == 0 {
			RetErrorWithMessage("无该用户")
		}
		balance := strconv.Itoa(seller[0].Money)
		return Ret{
			Status: "ok",
			Rets:   []string{balance},
		}
	}
	if NUM == "1" {
		var buyer []BUYERS
		err := Db.Select(&buyer, "select * from buyers where id =?;", user_id)
		if err != nil {
			panic(err)
		}
		if len(buyer) == 0 {
			RetErrorWithMessage("无该用户")
		}
		balance := strconv.Itoa(buyer[0].Money)
		return Ret{
			Status: "ok",
			Rets:   []string{balance},
		}
	}
	return Ret{
		Status: "err",
		Rets:   nil,
	}
}

//生成UUID（订单号）
func creatUUID() string {
	u, _ := uuid.NewV4()
	return u.String()
}

func Purchase(req Req) Ret { ///一次只能购买一种东西
	buyer_id := verifyToken(req.Token)
	for k := 0; k < len(req.Args); k++ {
		temp := strings.Split(req.Args[k], "\x1f")
		good_id, _ := strconv.Atoi(temp[0])
		number, _ := strconv.Atoi(temp[1])
		price, _ := strconv.Atoi(temp[2])
		coupon_id, _ := strconv.Atoi(temp[3])
		var good []GOODS
		Db.Select(&good, "select * from goods where id=?", good_id)
		if len(good) == 0 {
			RetErrorWithMessage("无此商品")
		}
		if number > good[0].Number {
			RetErrorWithMessage("超出库存")
		}
		seller_id := good[0].Seller_id
		var buyer []BUYERS
		Db.Select(&buyer, "select * from buyers where id=?", buyer_id)
		if len(buyer) == 0 {
			RetErrorWithMessage("没有该用户")
		}
		if buyer[0].Money-number < 0 {
			RetErrorWithMessage("余额不足")
		}
		timestamp := time.Now().Unix()
		_, err := Db.Exec("begin ;")
		if err != nil {
			panic(err)
		}
		_, err = Db.Exec("update buyers set money = money - ? where id= ?", price, buyer_id)
		if err != nil {
			Db.Exec("ROLLBACK")
			panic(err)
		}
		_, err = Db.Exec("update sellers set adv_payment = adv_payment + ? where id = ?;", price, seller_id)
		if err != nil {
			Db.Exec("ROLLBACK")
			panic(err)
		}
		_, err = Db.Exec("insert into trades(seller_id,buyer_id,good_id,price,number,status,time,UUID,transport_order) values(?,?,?,?,?,'waiting',?,?,'');", seller_id, buyer_id, good_id, price, number, timestamp, creatUUID())
		if err != nil {
			Db.Exec("ROLLBACK")
			panic(err)
		}
		_, err = Db.Exec("update goods set amount = amount - ? where id = ?", number, good_id)
		if err != nil {
			Db.Exec("ROLLBACK")
			panic(err)
		}
		_, err = Db.Exec("delete from coupons where buyers_id = ? and coupon_id = ?", buyer_id, coupon_id)
		if err != nil {
			Db.Exec("ROLLBACK")
			panic(err)
		}
		Db.Exec("commit;")
	}
	temp := strings.Split(req.Args[0], "\x1f")
	var good []GOODS
	Db.Select(&good, "select * from goods where id = ?;", temp[0])
	inventory := strconv.Itoa(good[0].Number)
	return Ret{
		Status: "ok",
		Rets:   []string{inventory},
	}
}

func Scanbuy(req Req) Ret {
	var trade []TRADES
	buyer_id := verifyToken(req.Token)
	err := Db.Select(&trade, "select * from trades where buyer_id=?", buyer_id)
	if err != nil {
		panic(err)
	}
	tocode := make([]string, len(trade), len(trade)+5)
	for i := 0; i < len(trade); i++ {
		var good []GOODS
		err = Db.Select(&good, "select * from goods where id=?", trade[i].Good_id)
		if err != nil {
			panic(err)
		}
		var seller []SELLERS
		err = Db.Select(&seller, "select * from sellers where id=?", trade[i].Seller_id)
		if err != nil {
			panic(err)
		}
		tocode[i] = strconv.Itoa(trade[i].ID) + "\x1f" + trade[i].Time + "\x1f" + good[0].Name + "\x1f" + strconv.Itoa(trade[i].Number) + "\x1f" + strconv.Itoa(trade[i].Price) + "\x1f" + seller[0].Name + "\x1f" + trade[i].Status + "\x1f" + trade[i].UUID + "\x1f" + trade[i].Transport_order
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

func Scantrade(req Req) Ret {
	var trade []TRADES
	seller_id := verifyToken(req.Token)
	Db.Select(&trade, "select *from trades where seller_id=?", seller_id)
	tocode := make([]string, len(trade), len(trade)+5)
	for i := 0; i < len(trade); i++ {
		var good []GOODS
		Db.Select(&good, "select * from goods where id=?;", trade[i].Good_id)
		var buyer []BUYERS
		Db.Select(&buyer, "select * from buyers where id=?;", trade[i].Buyer_id)
		tocode[i] = strconv.Itoa(trade[i].ID) + "\x1f" + trade[i].Time + "\x1f" + good[0].Name + "\x1f" + strconv.Itoa(trade[i].Number) + "\x1f" + strconv.Itoa(trade[i].Price) + "\x1f" + buyer[0].Name + "\x1f" + trade[i].Status + "\x1f" + trade[i].UUID + "\x1f" + trade[i].Transport_order
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

func Delivery(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	trade_id, _ := strconv.Atoi(temp[0])
	transporting_order := temp[1]
	var trade []TRADES
	Db.Select(&trade, "select *from trades where id=?", trade_id)
	if len(trade) == 0 {
		RetErrorWithMessage("查无此交易")
	}
	if trade[0].Status == "waiting" {
		Db.Exec("update trades set status='transporting' where id=?", trade_id)
		Db.Exec("update trades set transport_order=? where id=?", transporting_order, trade_id)
	} else {
		RetErrorWithMessage("该商品已发货")
	}
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

func Returngood(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	trade_id, _ := strconv.Atoi(temp[0])
	var trade []TRADES
	Db.Select(&trade, "select *from trades where id=?", trade_id)
	if len(trade) == 0 {
		RetErrorWithMessage("查无此交易")
	}
	if trade[0].Status == "waiting" || trade[0].Status == "transporting" {
		Db.Exec("update trades set status='return_require' where id=?", trade_id)
	} else {
		if trade[0].Status == "succeed" {
			RetErrorWithMessage("您已确认收货，不能退货")
		} else if trade[0].Status == "return_require" {
			RetErrorWithMessage("已申请，不必重复申请")
		} else if trade[0].Status == "returning" {
			RetErrorWithMessage("正在退货中")
		} else if trade[0].Status == "return_succeed" {
			RetErrorWithMessage("您已退货成功")
		}
	}
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

func Surereturn(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	trade_id, _ := strconv.Atoi(temp[0])
	var trade []TRADES
	Db.Select(&trade, "select *from trades where id=?", trade_id)
	if len(trade) == 0 {
		RetErrorWithMessage("查无此交易")
	}
	if trade[0].Status == "return_require" {
		Db.Exec("update trades set status='returning' where id=?", trade_id)
	} else {
		RetErrorWithMessage("用户当前无申请或您已处理")
	}
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

func Surereturnsucceed(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	trade_id, _ := strconv.Atoi(temp[0])
	var trade []TRADES
	Db.Select(&trade, "select *from trades where id=?", trade_id)
	if len(trade) == 0 {
		RetErrorWithMessage("查无此交易")
	}
	if trade[0].Status == "returning" {
		Db.Exec("update trades set status='return_succeed' where id=?", trade_id)
	} else {
		RetErrorWithMessage("用户当前无申请或您已处理")
	}
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

func Surereceive(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	trade_id, _ := strconv.Atoi(temp[0])
	var trade []TRADES
	Db.Select(&trade, "select *from trades where id=?", trade_id)
	if len(trade) == 0 {
		RetErrorWithMessage("查无此交易")
	}
	if trade[0].Status == "transporting" {
		Db.Exec("update trades set status='succeed' where id=?", trade_id)
	} else {
		RetErrorWithMessage("您当前的货物状态不允许您确认收货")
	}
	return Ret{
		Status: "ok",
		Rets:   []string{},
	}
}

func Search(req Req) Ret {
	temp := req.Args[0]
	str2 := "%" + temp + "%"
	log.Println(str2)
	var good []GOODS
	err := Db.Select(&good, "select * from goods where name like ?", str2)
	if err != nil {
		panic(err)
	}
	tocode := make([]string, len(good), len(good)+5)
	for i := 0; i < len(good); i++ {
		tocode[i] = strconv.Itoa(good[i].Id) + "\x1f" + good[i].Name + "\x1f" + strconv.Itoa(good[i].Price) + "\x1f" + good[i].Image
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

func Searchself(req Req) Ret {
	seller_id := verifyToken(req.Token)
	var good []GOODS
	err := Db.Select(&good, "select * from goods where seller_id=?", seller_id)
	if err != nil {
		panic(err)
	}
	tocode := make([]string, len(good), len(good)+5)
	for i := 0; i < len(good); i++ {
		tocode[i] = strconv.Itoa(good[i].Id) + "\x1f" + good[i].Name + "\x1f" + strconv.Itoa(good[i].Price) + "\x1f" + good[i].Image
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

func discount_method(input string) string {
	if input[0] == 'o' {
		input = input[4:]
		discount, _ := strconv.ParseFloat(input, 64)
		discount = discount * 10
		return "打" + strconv.FormatFloat(discount, 'f', -1, 64) + "折"
	} else if input[0] == 's' {
		input = input[6:]
		str := strings.Split(input, "-")
		return "满" + str[0] + "减" + str[1]
	}
	return "未识别类型"
}

func Scancoupon(req Req) Ret {
	var coupon []COUPONS
	buyer_id := verifyToken(req.Token)
	err := Db.Select(&coupon, "select * from coupons where buyers_id = ? ;", buyer_id)
	if err != nil {
		RetErrorWithMessage("您没有任何卡券")
	}
	tocode := make([]string, len(coupon), len(coupon)+5)
	for i := 0; i < len(coupon); i++ {
		var coupon_type []COUPON_TYPE
		err = Db.Select(&coupon_type, "select * from coupons_type where id =?;", coupon[i].Coupon_id)
		if err != nil {
			RetErrorWithMessage("优惠券信息查询失败")
		}
		if len(coupon) != 0 {
			if coupon_type[0].IsAvailable == 1 {
				var seller []SELLERS
				err = Db.Select(&seller, "select * from sellers where id =?;", coupon_type[0].Seller_id)
				if err != nil {
					RetErrorWithMessage("商家获取失败")
				}
				if len(seller) == 0 {
					RetErrorWithMessage("不存在该商家")
				}
				var GoodName string
				var Good_id int
				if coupon_type[0].Good_id == 0 {
					GoodName = "通用"
				} else {
					var good []GOODS
					err = Db.Select(&good, "select * from goods where id =?", coupon_type[0].Good_id)
					if err != nil {
						RetErrorWithMessage("商品信息获取失败")
					}
					GoodName = good[0].Name
					Good_id = good[0].Id
				}
				tocode[i] = strconv.Itoa(coupon_type[0].Id) + "\x1f" + seller[0].Name + "\x1f" + GoodName + "\x1f" + discount_method(coupon_type[0].Others) + "\x1f" + coupon_type[0].Deadline + "\x1f" + strconv.Itoa(Good_id)
			}
		}
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

func Clickinformation(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	good_id := temp[0]
	var good []GOODS
	err := Db.Select(&good, "select * from goods where id =?", good_id)
	if err != nil {
		panic(err)
	}
	if len(good) == 0 {
		RetErrorWithMessage("查无此商品")
	}
	tocode := make([]string, len(good), len(good)+5)
	for i := 0; i < len(good); i++ {
		tocode[i] = strconv.Itoa(good[i].Seller_id) + "\x1f" + good_id + "\x1f" + good[i].Name + "\x1f" + good[i].Introduce + "\x1f" + strconv.Itoa(good[i].Price) + "\x1f" + strconv.Itoa(good[i].Number) + "\x1f" + good[i].Others + "\x1f" + good[i].Image
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

func Scansellercoupon(req Req) Ret {
	temp := strings.Split(req.Args[0], "\x1f")
	seller_id := temp[0]
	good_id := temp[1]
	var coupon_type []COUPON_TYPE
	err := Db.Select(&coupon_type, "select * from coupons_type where (seller_id = ? and good_id= ?)", seller_id, good_id)
	if err != nil {
		panic(err)
	}
	if len(coupon_type) == 0 {
		RetErrorWithMessage("该商家无优惠券")
	}
	tocode := make([]string, len(coupon_type), len(coupon_type)+5)
	for i := 0; i < len(coupon_type); i++ {
		tocode[i] = strconv.Itoa(coupon_type[i].Id) + "\x1f" + discount_method(coupon_type[i].Others) + "\x1f" + strconv.Itoa(coupon_type[i].Amount)
	}
	return Ret{
		Status: "ok",
		Rets:   tocode,
	}
}

func main() {
	Tokens = make(map[string]int)
	connectDb()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, indexHTML)
	})

	var Debug bool = false
	var auth AuthFunc = func(r *Req) bool {
		if r.Method == "Login" {
			return true
		}
		exists := verifyToken(r.Token)
		if exists == -1 {
			return false
		}
		// details  token再次认证
		return true
	}

	handlers := make(map[string]HandleFunc) // map[method]..
	handlers["echo"] = Echo
	handlers["Login"] = Login
	handlers["Register"] = Register
	handlers["Setpreferential"] = Setpreferential
	handlers["Setcoupon"] = Setcoupon
	handlers["Puton"] = Puton
	handlers["Getcoupon"] = Getcoupon
	handlers["Putcart"] = Putcart
	handlers["Biringcart"] = Biringcart
	handlers["Charge"] = Charge
	handlers["Purchase"] = Purchase
	handlers["Scanbuy"] = Scanbuy
	handlers["Scantrade"] = Scantrade
	handlers["Delivery"] = Delivery
	handlers["Returngood"] = Returngood
	handlers["Search"] = Search
	handlers["Surereceive"] = Surereceive
	handlers["Surereturnsucceed"] = Surereturnsucceed
	handlers["Surereturn"] = Surereturn
	handlers["Scancart"] = Scancart
	handlers["Scancoupon"] = Scancoupon
	handlers["Scancharge"] = Scancharge
	handlers["Clickinformation"] = Clickinformation
	handlers["Scansellercoupon"] = Scansellercoupon
	handlers["Searchself"] = Searchself
	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		func() {
			defer func() {
				r := recover()
				if r == nil {
					return
				}

				log.Println(r)

				msg := GetMessageFromError(r)
				w.WriteHeader(http.StatusOK)
				if Debug {
					log.Println("err:", r)
				}
				WriteError(w, errors.New(msg))
				return
			}()
			var req Req
			err := json.NewDecoder(r.Body).Decode(&req) //err := json.NewDecoder(r.Body).Decode(&req)
			if err != nil {
				if Debug {
					RetErrorWithMessage(err.Error())
				}
				panic(err)
			} /*test*/

			if auth(&req) == false {
				w.WriteHeader(http.StatusOK)
				ret := Ret{
					Status: "auth",
					Rets:   []string{"登陆失效，请重新登陆"},
				}

				json.NewEncoder(w).Encode(&ret) //写入web
				return
			}
			handler := handlers[req.Method]
			if handler == nil {
				if Debug {
					RetErrorWithMessage("not define message: " + req.Method)
				}
				panic("not define message: " + req.Method)
				return
			}

			ret := handler(req)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(&ret)
		}()

	})

	err := http.ListenAndServe(":8081", nil)
	if err != nil {
		log.Fatal(err)
	}

}
