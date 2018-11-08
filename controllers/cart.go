package controllers

import (
    "github.com/astaxie/beego"
    "github.com/astaxie/beego/orm"
    "DailyFresh/models"
    "github.com/gomodule/redigo/redis"
    "strconv"
)

type CartController struct {
    beego.Controller
}

/** 封装获取购物车商品数量的方法 */
func GetCartCount(this *beego.Controller) (cartCount int) {
    userName := GetUser(this)

    if userName != "" {
        cartCount = 0
    }
    // 连接Redis
    conn, err := redis.Dial("tcp", ":6379")
    if err != nil {
        beego.Error("redis link error", err)
    }
    defer conn.Close()

    // 获取用户信息
    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    o.Read(&user, "Name")

    // 从Redis中获取数据
    rep, err := conn.Do("hlen", "cart_"+strconv.Itoa(user.Id))
    cartCount, err = redis.Int(rep, err)
    this.Data["cartCount"] = cartCount

    return
}

/** 添加购物车 */
func (this *CartController) HandleAddCart() {
    skuid, err1 := this.GetInt("skuid")
    count, err2 := this.GetInt("count")
    // 响应数据结构体，一个map集合，后续转换成Json返回s
    resp := make(map[string]interface{})
    defer this.ServeJSON()
    // 判断数据转换是否异常
    if err1 != nil || err2 != nil {
        resp["code"] = 1
        resp["msg"] = "传递的数据不正确"
        this.Data["json"] = resp
        return
    }
    beego.Info("count = ", count)
    userName := GetUser(&this.Controller)
    if userName == "" {
        resp["code"] = 2
        resp["msg"] = "用户未登录"
        this.Data["json"] = resp
        return
    }
    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    o.Read(&user, "Name")

    // 更新redis中存储的数据
    conn, err := redis.Dial("tcp", ":6379")
    if err != nil {
        beego.Error("redis 连接失败s")
        return
    }
    defer conn.Close()
    if conn != nil {
        // 先获取原来添加的数据
        preCount, err := redis.Int(conn.Do("hget", "cart_"+strconv.Itoa(user.Id), skuid))
        beego.Info("preCount = ", preCount)
        // 新添加数量，加上之前保存的，重新设置覆盖数据
        conn.Do("hset", "cart_"+strconv.Itoa(user.Id), skuid, count+preCount)

        // 获取购物车总数量
        rep, err := conn.Do("hlen", "cart_"+strconv.Itoa(user.Id))
        // 通过回复助手函数转换
        cartCount, _ := redis.Int(rep, err)
        beego.Info("cartCount", cartCount)
        resp["cartCount"] = cartCount
    }

    resp["code"] = 5
    resp["msg"] = "ok"
    beego.Info("data", resp)
    this.Data["json"] = resp

}

func (this *CartController) ShowCart() {
    userName := GetUser(&this.Controller)

    conn, err := redis.Dial("tcp", ":6379")
    if err != nil {
        beego.Error("redis link error ", err)
        return
    }
    defer conn.Close()

    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    o.Read(&user, "Name")

    goodsMap, _ := redis.IntMap(conn.Do("hgetall", "cart_"+strconv.Itoa(user.Id)))

    goods := make([]map[string]interface{}, len(goodsMap))

    i := 0
    totalPrice := 0 // 总价
    totalCount := 0 // 总数量
    //  遍历从Reids中获取的订单数据
    for index, value := range goodsMap {
        skuid, _ := strconv.Atoi(index)
        var goodsSku models.GoodsSKU
        // 获取商品数据
        goodsSku.Id = skuid
        o.Read(&goodsSku)

        temp := make(map[string]interface{})
        temp["goodsSku"] = goodsSku
        temp["count"] = value
        temp["addPrice"] = goodsSku.Price * value

        // 计算总价和总数量
        totalPrice += goodsSku.Price * value
        totalCount += value

        goods[i] = temp
        i += 1
    }
    // 返回视图
    this.Data["goods"] = goods
    this.Data["totalPrice"] = totalPrice
    this.Data["totalCount"] = totalCount

    this.TplName = "cart.html"
}

func (this *CartController) HandleUpdateCart() {
    skuid, err1 := this.GetInt("skuid")
    count, err2 := this.GetInt("count")
    resp := make(map[string]interface{})
    defer this.ServeJSON()
    if err1 != nil || err2 != nil {
        resp["code"] = 1
        resp["msg"] = "请求数据不正确"
        this.Data["json"] = resp
        return
    }

    userName := GetUser(&this.Controller)

    if userName == "" {
        resp["code"] = 2
        resp["msg"] = "用户示登录"
        this.Data["json"] = resp
        return
    }

    conn, err := redis.Dial("tcp", ":6379")

    if err != nil {
        resp["code"] = 3
        resp["msg"] = "redis 连接失败"
        this.Data["json"] = resp
        beego.Error("redis link error ", err)
        return
    }
    defer conn.Close()

    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    o.Read(&user, "Name")
    conn.Do("hset", "cart_"+strconv.Itoa(user.Id), skuid, count)

    resp["code"] = 5
    resp["msg"] = "ok"
    this.Data["json"] = resp
}

func (this *CartController) HandleDeteleCart() {
    // 获取商品Id
    skuid,err := this.GetInt("skuid")
    resp := make(map[string]interface{})
    defer this.ServeJSON()
    // 校验数据
    if err != nil {
        resp["code"] = 1
        resp["msg"] = "请求数据错误"
        this.Data["json"] = resp
        return
    }

    // 获取用户名，在方法内部做了强转string了
    userName := GetUser(&this.Controller)
    if userName == "" {
        resp["code"] = 2
        resp["msg"] = "用户示登录"
        this.Data["json"] = resp
        return
    }

    // 连接Redis
    conn,err := redis.Dial("tcp",":6379")
    if err != nil{
        resp["code"] = 3
        resp["msg"] = "Redis 连接失败"
        this.Data["json"] = resp
        return
    }
    defer conn.Close()

    // 根据用户名，获取用户ID
    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    o.Read(&user,"Name")
    // 删除Redis中对应的商品数据
    conn.Do("hdel","cart_"+strconv.Itoa(user.Id),skuid)

    // 返回结果
    resp["code"] = 5
    resp["msg"] = "ok"
    this.Data["json"] = resp

}
