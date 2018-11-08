package routers

import (
	"DailyFresh/controllers"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/context"
)

func init() {

	beego.InsertFilter("/user/*",beego.BeforeExec,FilterFunc)
	// 首页
    beego.Router("/", &controllers.GoodsController{},"get:ShowIndex")

    beego.Router("/login",&controllers.UserController{},"get:ShowLogin;post:HandleLogin")
    beego.Router("/register",&controllers.UserController{},"get:ShowRegister;post:HandleRegister")
	// 用户激活
    beego.Router("/active",&controllers.UserController{},"get:ActiveUser")
	// 用户退出
	beego.Router("/user/logout",&controllers.UserController{},"get:Logout")
	// 用户中心信息页
    beego.Router("/user/userCenterInfo",&controllers.UserController{},"get:ShowUserCenterInfo")
	// 用户中心订单页
	beego.Router("/user/userCenterOrder",&controllers.UserController{},"get:ShowUserCenterOrder")
	// 用户中心地址页
	beego.Router("/user/userCenterSite",&controllers.UserController{},"get:ShowUserCenterSite;post:HandleUserCenterSite")
	// 添加购物车
	beego.Router("/user/addCart",&controllers.CartController{},"post:HandleAddCart")
	// 显示购物车界面
	beego.Router("/user/cart",&controllers.CartController{},"get:ShowCart")
	// 更新购物车数量
	beego.Router("/user/updateCart",&controllers.CartController{},"post:HandleUpdateCart")
	// 删除购物车商品
	beego.Router("/user/deleteCart",&controllers.CartController{},"post:HandleDeteleCart")
	// 显示订单界面
	beego.Router("/user/showOrder",&controllers.OrderController{},"post:ShowOrder")
	// 用户提交订单
	beego.Router("/user/addOrder",&controllers.OrderController{},"post:HandleAddOrder")
	// 订单支付
	beego.Router("/user/pay",&controllers.OrderController{},"get:HandlePay")
	// 支付结果处理
	beego.Router("/user/prySuccess",&controllers.OrderController{},"get:HandlePaySuccess")

	// 商品详情
	beego.Router("/goodsDetail",&controllers.GoodsController{},"get:ShowGoodsDetail")
	// 商品列表
	beego.Router("/goodsList",&controllers.GoodsController{},"get:ShowGoodsList")
	// 搜索商品
	beego.Router("/goodsSearch",&controllers.GoodsController{},"get:ShowSearch;post:HandleSearch")

}

var FilterFunc = func(ctx *context.Context) {
	beego.Info("route 拦截")
	userName := ctx.Input.Session("userName")
	if userName == nil{
		ctx.Redirect(302,"/login")
		return
	}
}