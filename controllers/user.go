package controllers

import (
    "github.com/astaxie/beego"
    "regexp"
    "github.com/astaxie/beego/orm"
    "DailyFresh/models"
    "github.com/astaxie/beego/utils"
    "strconv"
    "encoding/base64"
    "github.com/gomodule/redigo/redis"
    "math"
)

type UserController struct {
    beego.Controller
}

/** 显示用户登录界面 */
func (this *UserController) ShowLogin() {
    temp := this.Ctx.GetCookie("userName")

    userName, _ := base64.StdEncoding.DecodeString(temp)

    if string(userName) == "" {
        this.Data["userName"] = ""
        this.Data["checked"] = ""
    } else {
        this.Data["userName"] = string(userName)
        this.Data["checked"] = "checked"
    }

    this.TplName = "login.html"
}

/** 处理用户登录请求 */
func (this *UserController) HandleLogin() {
    // 获取数据
    userName := this.GetString("username")
    pwd := this.GetString("pwd")
    // 校验数据
    if userName == "" || pwd == "" {
        this.Data["errmsg"] = "用户名或密码为空，请重新输入"
        this.TplName = "login.html"
        return
    }

    // 处理数据
    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    err := o.Read(&user, "Name")

    if err != nil {
        this.Data["errmsg"] = "用户名输入错误，请重新输入"
        this.TplName = "login.html"
        return
    }

    if user.PassWord != pwd {
        this.Data["errmsg"] = "密码输入错误错误，请重新输入"
        this.TplName = "login.html"
        return
    }

    if user.Active != true {
        this.Data["errmsg"] = "用户未激活，请先前往邮箱激活用户"
        this.TplName = "login.html"
        return
    }

    remember := this.GetString("remember")

    if remember == "on" {
        temp := base64.StdEncoding.EncodeToString([]byte(userName))

        this.Ctx.SetCookie("userName", temp, 24*3600*30)
    } else {
        this.Ctx.SetCookie("userName", userName, -1)
    }

    this.SetSession("userName", userName)
    this.Redirect("/", 302)

    // 返回视图
    //this.Ctx.WriteString("用户登录成功")
}

/** 显示用户注册界面 */
func (this *UserController) ShowRegister() {
    this.TplName = "register.html"
}

/** 处理用户注册请求 */
func (this *UserController) HandleRegister() {
    // 获取数据
    userName := this.GetString("user_name")
    pwd := this.GetString("pwd")
    cpwd := this.GetString("cpwd")
    email := this.GetString("email")
    // 检验数据
    if userName == "" || pwd == "" || cpwd == "" || email == "" {
        this.Data["errmsg"] = "数据不完整，请重新注册~"
        this.TplName = "register.html"
        return
    }

    if pwd != cpwd {
        this.Data["errmsg"] = "两次密码输入不一至，请重新注册~"
        this.TplName = "register.html"
        return
    }

    reg, _ := regexp.Compile("^[A-Za-z0-9\u4e00-\u9fa5]+@[a-zA-Z0-9_-]+(\\.[a-zA-Z0-9_-]+)+$")
    res := reg.FindString(email)
    if res == "" {
        this.Data["errmsg"] = "邮箱格式不正确，请重新注册~"
        this.TplName = "register.html"
        return
    }
    // 处理数据

    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    user.PassWord = pwd
    user.Email = email
    _, err := o.Insert(&user)

    if err != nil {

        this.Data["errmsg"] = "用户名已被注册，请重新注册~"
        this.TplName = "register.html"
        return
    }

    // uesfwcyqegzfbcgf 发送激活邮件
    config := `{"username":"974912946@qq.com","password":"uesfwcyqegzfbcgf","host":"smtp.qq.com","port":587}`
    emailConn := utils.NewEMail(config)
    emailConn.From = "974912946@qq.com"
    emailConn.To = []string{email}
    emailConn.Subject = "天天生鲜用户注册激活"
    emailConn.Text = "192.168.1.5:8080/active?id=" + strconv.Itoa(user.Id)
    emailConn.Send()

    // 返回视图

    //this.Redirect("/login", 302)
    this.Ctx.WriteString("注册成功，请去邮箱激活用户！")
}

/** 用户激活 */
func (this *UserController) ActiveUser() {
    id, err := this.GetInt("id")

    if err != nil {
        this.Data["errmsg"] = "要激活的用户不存在"
        this.TplName = "register.html"
        return
    }
    // 处理数据
    o := orm.NewOrm()
    var user models.User
    user.Id = id
    err = o.Read(&user)
    if err != nil {
        this.Data["errmsg"] = "要激活的用户不存在"
        this.TplName = "register.html"
        return
    }
    user.Active = true
    o.Update(&user)

    this.Redirect("/login", 302)
}

/** 用户退出 */
func (this *UserController) Logout() {
    this.DelSession("userName")
    this.Redirect("/login", 302)
}

/** 显示用户中心信息页 */
func (this *UserController) ShowUserCenterInfo() {
    userName := GetUser(&this.Controller)

    // 查询用户的默认地址
    o := orm.NewOrm()
    var addr models.Address
    o.QueryTable("Address").RelatedSel("User").Filter("User__Name", userName).Filter("IsDefault", true).One(&addr)

    if addr.Id == 0 {
        this.Data["addr"] = ""
    } else {
        this.Data["addr"] = addr
    }

    // 获取Redis连接
    conn, err := redis.Dial("tcp", ":6379")

    if err != nil {
        beego.Error("redis 连接失败 : ", err)
    }
    var goodsSKUs []models.GoodsSKU
    if conn != nil {
        defer conn.Close()
        var user models.User
        user.Name = userName
        o.Read(&user, "Name")
        // 根据用户Id从Redis中获取用户浏览记录
        rep, err := conn.Do("lrange", "history_"+strconv.Itoa(user.Id), 0, 4)
        goodsIDs, _ := redis.Ints(rep, err)

        // 遍历商品Id切片，查询商品信息
        for _, value := range goodsIDs {
            var goods models.GoodsSKU
            goods.Id = value
            o.Read(&goods)
            goodsSKUs = append(goodsSKUs, goods)
        }
    }

    beego.Info("size", len(goodsSKUs))

    this.Data["goodsSKUs"] = goodsSKUs
    ShowUserLayout(&this.Controller)
    this.TplName = "user_center_info.html"
}

/** 显示用户中心订单页 */
func (this *UserController) ShowUserCenterOrder() {

    userName := GetUser(&this.Controller)

    pageIndex, err := this.GetInt("pageIndex")

    if err != nil {
        pageIndex = 1
    }
    o := orm.NewOrm()
    var user models.User
    user.Name = userName
    o.Read(&user, "Name")

    count, _ := o.QueryTable("OrderInfo").RelatedSel("User").Filter("User__Id", user.Id).OrderBy("-Time").Count()
    pageSize := 2
    pageCount := math.Ceil(float64(count) / float64(pageSize)) // count/pageSize

    pages := pageTool(int(pageCount), pageIndex)

    this.Data["pages"] = pages
    this.Data["pageIndex"] = pageIndex

    prePage := pageIndex - 1
    if prePage <= 1 {
        prePage = 1
    }
    this.Data["prePage"] = prePage

    nextPage := pageIndex + 1
    if nextPage > int(pageCount) {
        nextPage = int(pageCount)
    }
    this.Data["nextPage"] = nextPage

    // 计算数据获取位置
    start := (pageIndex - 1) * pageSize

    // 根据用户先查询订单信息
    var orderInfs []models.OrderInfo
    // 降序排列查询订单信息
    o.QueryTable("OrderInfo").RelatedSel("User").Filter("User__Id", user.Id).OrderBy("-Time").Limit(pageSize,start).All(&orderInfs)
    // 返回封装订单信息的切片
    goodsBuffer := make([]map[string]interface{}, len(orderInfs))

    beego.Info("len",len(orderInfs))

    // 遍历获取对应的订单商品信息
    for index, orderInfo := range orderInfs {
        temp := make(map[string]interface{})
        // 订单商品切片
        var orderGoods []models.OrderGoods
        o.QueryTable("OrderGoods").RelatedSel("OrderInfo", "GoodsSKU").Filter("OrderInfo__Id", orderInfo.Id).All(&orderGoods)

        temp["orderInfo"] = orderInfo
        temp["grderGoods"] = orderGoods
        goodsBuffer[index] = temp
    }

    this.Data["goodsBuffer"] = goodsBuffer
    ShowUserLayout(&this.Controller)
    this.TplName = "user_center_order.html"
}

/** 显示用户中心地址页 */
func (this *UserController) ShowUserCenterSite() {
    userName := GetUser(&this.Controller)

    o := orm.NewOrm()
    var addr models.Address

    o.QueryTable("Address").RelatedSel("User").Filter("User__Name", userName).Filter("Isdefault", true).One(&addr)

    // 返回视图
    this.Data["addr"] = addr
    ShowUserLayout(&this.Controller)
    this.TplName = "user_center_site.html"
}

/**处理用户中心添加地址请求*/
func (this *UserController) HandleUserCenterSite() {
    receiver := this.GetString("receiver")
    addr := this.GetString("addr")
    zipCode := this.GetString("zipCode")
    phone := this.GetString("phone")

    if receiver == "" || addr == "" || zipCode == "" || phone == "" {
        beego.Error("添加的地址数据不完")
        this.Redirect("/user/userCenterSite", 302)
        return
    }

    o := orm.NewOrm()
    var addrUser models.Address

    addrUser.Isdefault = true
    // 这里先根据默认地址查询数据，如果有数据，则把之前的默认地址更新成非默认地址
    err := o.Read(&addrUser, "Isdefault")
    beego.Error("err", err)
    // 判断返回的结果，更新成非默认地址
    if err == nil {
        addrUser.Isdefault = false
        o.Update(&addrUser)
    }

    // 查询用户信息，关联
    var user models.User
    userName := GetUser(&this.Controller)
    user.Name = userName
    o.Read(&user, "Name")
    beego.Info("id", user.Id, "name", userName)

    // 前面更新默认地址时，对象有赋值了ID，所以不能用原有的对象操作了
    // 新建一个对象，重新赋值插入数据库
    var addrUserNew models.Address
    addrUserNew.Receiver = receiver
    addrUserNew.Zipcode = zipCode
    addrUserNew.Phone = phone
    addrUserNew.Addr = addr
    addrUserNew.Isdefault = true // 设置默认地址为true
    addrUserNew.User = &user     // 设置用户信息

    id, err := o.Insert(&addrUserNew)

    beego.Info("addr id", id)
    beego.Error("addr err ", err)

    this.Redirect("/user/userCenterSite", 302)
}

func ShowUserLayout(this *beego.Controller) {
    GetUser(this)
    this.Layout = "user_center_layout.html"
}
