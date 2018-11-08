package controllers

import (
    "github.com/astaxie/beego"
    "github.com/astaxie/beego/orm"
    "DailyFresh/models"
    "github.com/gomodule/redigo/redis"
    "strconv"
    "strings"
    "time"
    "github.com/smartwalle/alipay"
    "fmt"
)

type OrderController struct {
    beego.Controller
}

/** 显示订单界面 */
func (this *OrderController) ShowOrder() {
    skuids := this.GetStrings("skuid")
    // 判断商品Id切片长度
    if len(skuids) == 0 {
        beego.Info("请求数据错误")
        this.Redirect("/user/cart", 302)
        return
    }
    beego.Info("skuids: ", skuids)
    conn, err := redis.Dial("tcp", ":6379")

    if err != nil {
        beego.Error("redis link error ", err)
        this.Redirect("/user/cart", 302)
        return
    }
    // 获取用户信息
    userName := GetUser(&this.Controller)

    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    o.Read(&user, "Name")

    // 订单商品信息切片
    goodsBuffer := make([]map[string]interface{}, len(skuids))

    totalPrice := 0 // 商品总价
    totalCount := 0 // 商品总数量

    for index, skuid := range skuids {
        // 存放单条商品信息的
        temp := make(map[string]interface{})
        // 字符类型Id转换成int类型
        id, _ := strconv.Atoi(skuid)
        var goodsSku models.GoodsSKU
        goodsSku.Id = id
        o.Read(&goodsSku)
        temp["goods"] = goodsSku
        // 获取商品数量
        count, _ := redis.Int(conn.Do("hget", "cart_"+strconv.Itoa(user.Id), id))
        temp["count"] = count
        // 计算当前商品小计价格
        amount := goodsSku.Price * count
        temp["amount"] = amount

        totalPrice += amount
        totalCount += count
        goodsBuffer[index] = temp
    }
    // 传递订单商品
    this.Data["goodsBuffer"] = goodsBuffer
    // 获取用户地址
    var addrs []models.Address
    o.QueryTable("Address").RelatedSel("User").Filter("User__Id", user.Id).All(&addrs)
    this.Data["addrs"] = addrs

    // 传递总价和总件数
    this.Data["totalPrice"] = totalPrice
    this.Data["totalCount"] = totalCount
    transferPrice := 10 // 运费
    this.Data["transferPrice"] = transferPrice
    // 传递加运费后的总价
    this.Data["realyPrice"] = totalPrice + transferPrice
    // 传递所有商品的Id
    this.Data["skuids"] = skuids

    this.TplName = "place_order.html"
}

/*
  提交订单
  1、先获取数据
  2、校验数据
  3、处理数据
    3-1、需要处理库存问题
    3-2、多个用户下单过程
    3-3、下单失败，循环多次下单
    3-4、根据库存数据来处理用户是否下单成功
  4、返回结果
*/
func (this *OrderController) HandleAddOrder() {
    addrId, err1 := this.GetInt("addrId")
    payId, err2 := this.GetInt("payId")

    // 客户端传递过的数组，获取不到，获取的只是字符串，所以下面要处理一下
    skuid := this.GetString("skuids")
    transferPrice, err3 := this.GetInt("transferPrice")
    totalCount, err4 := this.GetInt("totalCount")
    realyPrice, err5 := this.GetInt("realyPrice")

    // 把[]截取掉
    ids := skuid[1:len(skuid)-1]
    // 根据空格转换成字符串切片
    skuids :=strings.Split(ids," ")

    beego.Error("err = ",err1,err2,err3,err4,err5,)
    beego.Info(skuids)
    resp := make(map[string]interface{})
    defer this.ServeJSON()

    if len(skuids) == 0 || err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
        resp["code"] = 1
        resp["msg"] = "订单信息提交失败"
        this.Data["json"] = resp
        return
    }

    userName := GetUser(&this.Controller)
    o := orm.NewOrm()

    o.Begin() // 开启事务

    var user models.User
    user.Name = userName
    o.Read(&user, "Name")

    // 封装订单信息数据
    var order models.OrderInfo
    order.OrderId = time.Now().Format("2006010215030405") + strconv.Itoa(user.Id)
    order.User = &user
    order.Orderstatus = 1
    order.PayMethod = payId
    order.TotalCount = totalCount
    order.TotalPrice = realyPrice
    order.TransitPrice = transferPrice

    var addr models.Address
    addr.Id = addrId
    o.Read(&addr)
    order.Address = &addr
    // 插入订单信息
    o.Insert(&order)
    // 向订单商品表中插入数据
    conn, err := redis.Dial("tcp", ":6379")
    if err != nil {
        resp["code"] = 2
        resp["msg"] = "Redis 连接失败"
        this.Data["json"] = resp
        o.Rollback() // 回滚事务
        return
    }
    defer conn.Close()

    for _, skuid := range skuids {
        id, _ := strconv.Atoi(skuid)
        var goods models.GoodsSKU
        // 获取商品信息
        goods.Id = id
        // 定义一个临时变量，订单提交失败，就提交3次。
        i := 3
        // 提交了三次还失败则返回失败，三次之内提交成功了，则成功
        for i > 0 {

            o.Read(&goods)

            // 订单商品
            var orderGoods models.OrderGoods
            orderGoods.OrderInfo = &order
            orderGoods.GoodsSKU = &goods

            // 从Redis获取对应的商品数量
            count, _ := redis.Int(conn.Do("hget", "cart_"+strconv.Itoa(user.Id), id))
            // 购买数量大于库存则返回
            if count > goods.Stock {
                resp["code"] = 3
                resp["msg"] = "商品库存数量不足"
                this.Data["json"] = resp
                o.Rollback() // 回滚事务
                return
            }

            orderGoods.Count = count
            orderGoods.Price = count * goods.Price

            // 插入订单商品数据
            o.Insert(&orderGoods)

            // ================= 根据商品库存量来处理了 ===============
            // 先临时保存一下商品的库存数量
            preCount := goods.Stock

            // 这里更新下商品对应的库存和销量，一个加,一个减
            goods.Stock -= count
            goods.Sales += count

            // 这里要更新数据库更新商品库存和商品销量，更新条件为商品Id和商品库存是否等于当前商品库存
           updateCount,_ := o.QueryTable("GoodsSKU").Filter("Id",goods.Id).Filter("Stock",preCount).Update(orm.Params{"Stock":goods.Stock,"sales":goods.Sales})

           // 数据库更新条数等于0，则表示更新失败
            if updateCount == 0 {
                // i大于0则继续循环提交
                if i > 0 {
                    // 标识减1，然后继续循环
                    i -= 1
                    continue
                }
                // 已经三次了，就是真的提交失败了，返回失败信息
                resp["code"] = 4
                resp["msg"] = "库存不足,订单提交失败"
                this.Data["json"] = resp
                o.Rollback() // 回滚事务
                return

            } else {
                // 订单提交成功，则删除Redis中的购物车数据
                conn.Do("hdel","cart_"+strconv.Itoa(user.Id),goods.Id)
                // 跳出循环了
                break
            }
        }
    }

    o.Commit() // 提交事务

    resp["code"] = 5
    resp["errmsg"] = "ok"
    this.Data["json"] = resp

}

/** 订单支付 */
func (this *OrderController) HandlePay() {
    orderId := this.GetString("orderId")
    totalPrice := this.GetString("totalPrice")
    appId := "2016091900547357"

    // 可选，支付宝提供给我们用于签名验证的公钥，通过支付宝管理后台获取
    var aliPublicKey = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAx2+C+0OLPVpnjAuWdyrHyPkemMtmEs9AnRDzvMoSzI3s6AHKyS7cijbe9CVVg1uHrKASjqT/8Q0KnaHL9yvYtS15uH4IUYjcPO8adB71a7oS+HB7r3imOfcGBA8AXpUmpKwjyn73669pXa+gz9diAQyYXBeXny1MWeXky9dCelxCWHEXKknXLuOWdHXvePRB+q99l+iLFruutfXMX2ItHkBcvIJcHNA7WJwyVRvmEqKk7W4sRheg/HvVz4rEropCc99tuNQNzV0Z/fnKwfN89mLNmyKqFYbuIpCjy/8gL0dbTEFZyzBCHKr74RTs7sbr/g/HtAnz+LVVXCV3DxR/CQIDAQAB"
    // 必须，上一步中使用 RSA签名验签工具 生成的私钥
    var privateKey = "MIIEpAIBAAKCAQEA20Yw7VsEM6Cdl4AezroeevkUgHuvUChoOc1z4ntP/7B+bhYy" +
    "86lykwNaBa8q1qx4fL82SxuFj5WiiDTtE3c4/hnwrCIz1RDyhoWucQx5Jlv8f//F" +
    "dkR244Ds5iXwZs6Ieu2ZLAYizUPgbwPDuPOp1WWnSo0GyQ48I0AoeSfIn6Opkx67" +
    "aDTu5tYgNF34z2ov57GqitMFvpuzs2J4MmXY6xORJTqjdA7UvVFgCW7Dr7YYIXwO" +
    "w82TCEscSE3RdyzzQXm43v/jTYlVZZH2XGuG7xbwD6gUD8n7mOPNRT0Kxq7HBebl" +
    "JMTY7iWIeWJ+h7PBvUQ9O55ZPaCJXeepg6VYpwIDAQABAoIBAQDaU8tHqkZGuXfw" +
    "b0s9fyf2PafiPkTSxUjxtNXb/fgrmKpqJoRZBLDmHII4Aq/ezB+z5hfDNQYJb25D" +
    "vJ8JsL34lA+E9REy5wr0UorcWRUP0qtZL2yHU6gk4iv/BGuXkbFm5MiMgxeH1jvT" +
    "jaYFs+e4aNznaAAHlLrgRnOGHsyt1UmFHYUPhc3ru/++5MDtIeLg/7fyshKJ5c1N" +
    "6QADpwkm6emzs3c3SS37HJ3Bcr3FHCl9ytu9fuWLDS5nvdFch8CPfdl0VcQinyEd" +
    "fH1qzW3EmnS5QdmtuAlJCBgE6+VBydyrerMmZhBjaD+WoXJtNs49WYJFUakSZbS/" +
    "ozAYoHVxAoGBAPJmbUuAINegfC2UF3qbfxZ7c5IXlGrVXUALbGpRHwnfuwqEVNt3" +
    "OHlg36XYkfFUaz5L4aAy/3p+YcZ2KsFXIGo1XdSoxF17DNCXx0YU6Y/rlvOKGRHM" +
    "C+4NbDmnax2ZWJXfWY3Q7wCUyYIONJM6zsTFoWBfL+IL/pNF9cl2fTb7AoGBAOeT" +
    "m8Pbg2JtmULjzF/cnBxmo1MDV1DlvkE8tZr2CJ+9SXluv17CK1Wz631z5Ck5IUly" +
    "mQV6sRmPFk/os7JYZX3yo2yQP3WwU6AFKBI+IMllN2c9ZvMzlIjOLa8rJ7N8GdY1" +
    "B+u6NWbS1TLnTZ0oJ0iKuyX+KbELTPVPvWaDdeVFAoGBAJaKMCRsjXj8rUItL6uw" +
    "eGwA/VRkmoMCwWft8EXS3YDnVqUAbCbkUslm9V5tMq367KOCwrwYD/wGEzkK2CC8" +
    "uF/dhsl0ioc3zUyahmKqyCbefCAByvH3lA0ifu0LYYW/X3msfVSKxnPI86B2rAYn" +
    "xpQD3OYaF4W+Rzs1fqDAmqETAoGANi7dVTg5R4BpSbNPEGbnx+Vj9XpkpbL7jvwL" +
    "adSDNAzv8g+tixhXV1gfk1zYV6TcWvkLQLLyWQ6Xo97InMP+CzgIcNBXaMv25QwP" +
    "0iTjOvwJuIgvXFwHNvM20TOBuIci7HHABrGs6QAPjjd8e3b3qgt7umn7i0cfnI4p" +
    "vKCppxkCgYBL1sgB/T8cEfieL3xVMJbQ4aHArT+T4IJzLauVQPB1ytzIM3w4xYlm" +
    "S7UvinPiAekS5iT8xL6xiBqPynydUbKvaKuAuzRhnp1fRU9VVMC85fIl/1XnhpHh" +
    "EhGCDJRMHcHHXppURQdfhnCy5bI0rFwZ0iCNEgGSkxpz1BebpOCIBA=="

    var client = alipay.New(appId, aliPublicKey, privateKey, false)

    var p = alipay.AliPayTradePagePay{}
    p.NotifyURL = "http://xxx"
    p.ReturnURL = "http://192.168.1.5:8080/user/prySuccess"
    p.Subject = "生鲜支付"
    p.OutTradeNo = orderId
    p.TotalAmount = totalPrice
    p.ProductCode = "FAST_INSTANT_TRADE_PAY"

    var url, err = client.TradePagePay(p)
    if err != nil {
        fmt.Println(err)
    }

    var payURL = url.String()
    // 支付结果跳转
    this.Redirect(payURL,302)

    fmt.Println(payURL)
    // 这个 payURL 即是用于支付的 URL，可将输出的内容复制，到浏览器中访问该 URL 即可打开支付页面。
}

func (this *OrderController)HandlePaySuccess()  {


// http://192.168.1.5/user/prySuccess?charset=utf-8
// out_trade_no=20181108170504581
// method=alipay.trade.page.pay.return
// total_amount=319.00
// sign=gT6AepUNvmwgQFeHxM84G8pABcr0k%2BIrXrTdaVe59jaPbktYgWWX2fJgfL3tqJzrKIt99Ah%2F7N4TSB5hn6KGH%2BW4JXdZI9fEXiHgU05hflCL1SYWY29iTPfnMACeNcK4IqIEyOUNJEdnHSQOUrrqPv%2Bk49qFqmDb2Z81nLJvPNgHvDQlhPHy8brsauQxDh95BN243ClzPVSXwdrWIUL1IQ5iC8QJSgFcvjykiGF2hCBrdxEmz%2FeCCYyi7L039n4VmDV3Wuhj%2B404GbSc9JgG3pSlSs%2FwSzqfjMr63z2P%2BYeEmDC3AgxJjNIjp9UuNwhmOwC0sQQcBeeynSMmgi2oNg%3D%3D
// trade_no=2018110822001493540200813959
// auth_app_id=2016091900547357
// version=1.0
// app_id=2016091900547357
// sign_type=RSA2
// seller_id=2088102176326677
// timestamp=2018-11-08+21%3A31%3A18

    orderId := this.GetString("out_trade_no")

    if orderId == "" {
        beego.Error("订单支付失败")
        this.Redirect("/user/userCenterOrder",302)
        return
    }

    o := orm.NewOrm()
    count,_:=o.QueryTable("OrderInfo").Filter("OrderId",orderId).Update(orm.Params{"Orderstatus":2})
    if count  == 0{
        beego.Error("更新订单信息失败")
        this.Redirect("/user/userCenterOrder",302)
        return
    }

    this.Redirect("/user/userCenterOrder",302)

}