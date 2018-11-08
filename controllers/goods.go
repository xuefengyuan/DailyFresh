package controllers

import (
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/orm"
	"DailyFresh/models"
	"github.com/gomodule/redigo/redis"
	"strconv"
	"math"
)

type GoodsController struct {
	beego.Controller
}

/**获取用户名 */
func GetUser(this *beego.Controller) string {
	userName := this.GetSession("userName")
	this.Data["imgUrl"] = "http://192.168.1.5:8888/" // 这个是图片地址的前缀，放在后台，后面修改IP的时候方便更新
	if userName == nil {
		this.Data["userName"] = ""
		return ""
	} else {
		this.Data["userName"] = userName.(string)

		return userName.(string)
	}
}



/** 显示主界面 */
func (this *GoodsController) ShowIndex() {
	GetUser(&this.Controller)
	o := orm.NewOrm()
	// 获取类型数据
	var goodsTypes []models.GoodsType
	o.QueryTable("GoodsType").All(&goodsTypes)
	this.Data["goodsTypes"] = goodsTypes

	// 获取轮播图数据
	var indexGoodsBanner []models.IndexGoodsBanner
	o.QueryTable("IndexGoodsBanner").OrderBy("Index").All(&indexGoodsBanner)
	this.Data["indexGoodsBanner"] = indexGoodsBanner

	// 获取促销端口数据
	var promotionGoods []models.IndexPromotionBanner
	o.QueryTable("IndexPromotionBanner").OrderBy("Index").All(&promotionGoods)
	this.Data["promotionGoods"] = promotionGoods

	// 首页商品展示数据
	goods := make([]map[string]interface{}, len(goodsTypes))

	// 往goods切片中的interface中插入类型数据
	for index, value := range goodsTypes {
		temp := make(map[string]interface{})
		temp["type"] = value
		goods[index] = temp
	}

	// 获取商品图片和文字数据
	for _, value := range goods {
		var textGoods []models.IndexTypeGoodsBanner
		var imgGoods []models.IndexTypeGoodsBanner
		// 关联商品类型，商品SKU，根据Index排序，过滤商品类型，和过滤商品文字或图片
		// 获取文字商品数据
		o.QueryTable("IndexTypeGoodsBanner").RelatedSel("GoodsType", "GoodsSKU").OrderBy("Index").Filter("GoodsType", value["type"]).Filter("DisplayType", 0).All(&textGoods)
		// 获取图片商品数据
		o.QueryTable("IndexTypeGoodsBanner").RelatedSel("GoodsType", "GoodsSKU").OrderBy("Index").Filter("GoodsType", value["type"]).Filter("DisplayType", 1).All(&imgGoods)

		value["textGoods"] = textGoods
		value["imgGoods"] = imgGoods
	}
	this.Data["goods"] = goods
	GetCartCount(&this.Controller)
	this.TplName = "index.html"
}

/** 商品详情 */
func (this *GoodsController) ShowGoodsDetail() {
	id, err := this.GetInt("id")
	if err != nil {
		beego.Info("获取商品id错误")
		this.Redirect("/", 302)
		return
	}

	o := orm.NewOrm()
	var goodsSku models.GoodsSKU
	goodsSku.Id = id
	//o.Read(&goodsSku)
	// 关联查询商品的类型
	o.QueryTable("GoodsSKU").RelatedSel("GoodsType", "Goods").Filter("Id", id).One(&goodsSku)
	// 获取详情界面的精品推荐
	var goodsNew []models.GoodsSKU
	// 关联商品类型表，根据类型过虑，按时间排序，通过limt获取2条数据
	o.QueryTable("GoodsSKU").RelatedSel("GoodsType").Filter("GoodsType", goodsSku.GoodsType).OrderBy("Time").Limit(2, 0).All(&goodsNew)

	userName := GetUser(&this.Controller)
	if userName != "" {
		o := orm.NewOrm()
		var user models.User
		user.Name = userName
		// 根据用户名查询用户
		o.Read(&user, "Name")
		// 添加历史浏览记录
		conn, err := redis.Dial("tcp", ":6379")
		if err != nil {
			beego.Error("Redis 连接错误", err)
		}

		if conn != nil {
			defer conn.Close()
			// 用list方式来存储，key,value方式，key为用户id value为商品id
			// 删除以前相同的历史浏览记录
			conn.Do("lrem", "history_"+strconv.Itoa(user.Id), 0, id)
			// 添加新的商品历史浏览记录
			conn.Do("lpush", "history_"+strconv.Itoa(user.Id), id)
		}

	}

	this.Data["goodsNew"] = goodsNew
	this.Data["goodsSku"] = goodsSku
	ShowLayout(&this.Controller)

	this.TplName = "detail.html"
}

/** 页码控制 */
func pageTool(pageCount, pageIndex int) []int {
	var pages []int
	if pageCount <= 5 { // 页码总数少于等于5页
		pages = make([]int, pageCount)
		for i, _ := range pages {
			pages[i] = i + 1
		}
	} else if pageIndex <= 3 { // 当前显示页，小于等于3
		pages = []int{1, 2, 3, 4, 5}
	} else if pageIndex > pageCount-3 { //当前显示页大于页码数减3
		pages = []int{pageCount - 4, pageCount - 3, pageCount - 2, pageCount - 1, pageCount}
	} else {
		pages = []int{pageIndex - 2, pageIndex - 1, pageIndex, pageIndex + 1, pageIndex + 2}
	}
	return pages
}

/** 显示商品列表 */
func (this *GoodsController) ShowGoodsList() {
	id, err := this.GetInt("typeId")
	if err != nil {
		beego.Error("请求路径错误")
		this.Redirect("/", 302)
		return
	}

	ShowLayout(&this.Controller)

	o := orm.NewOrm()
	var goodsType models.GoodsType
	goodsType.Id = id
	o.Read(&goodsType)

	//
	var goodsNew []models.GoodsSKU
	o.QueryTable("GoodsSKU").RelatedSel("GoodsType").Filter("GoodsType__Id", id).OrderBy("Time").Limit(2, 0).All(&goodsNew)

	// 获取商品
	var goods []models.GoodsSKU
	//o.QueryTable("GoodsSKU").RelatedSel("GoodsType").Filter("GoodsType__Id",id).All(&goods)
	// 分页实现
	// 获取对应分类的总条数
	count, _ := o.QueryTable("GoodsSKU").RelatedSel("GoodsType").Filter("GoodsType__Id", id).Count()
	pageSize := 2                                              // 每页取多少条
	pageCount := math.Ceil(float64(count) / float64(pageSize)) // 每页多少条// count/pageSize

	pageIndex, err := this.GetInt("pageIndex")
	if err != nil {
		pageIndex = 1
	}

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

	// 获取条数开始位置，当前下标页减1再乘以每页获取条数
	start := (pageIndex - 1) * pageSize

	// 按一定顺序获取商品，价格、人气等
	sort := this.GetString("sort")

	if sort == "sale" {
		o.QueryTable("GoodsSKU").RelatedSel("GoodsType").Filter("GoodsType__Id", id).OrderBy("Sales").Limit(pageSize, start).All(&goods)
	} else if sort == "price" {
		o.QueryTable("GoodsSKU").RelatedSel("GoodsType").Filter("GoodsType__Id", id).OrderBy("Price").Limit(pageSize, start).All(&goods)
	} else {
		o.QueryTable("GoodsSKU").RelatedSel("GoodsType").Filter("GoodsType__Id", id).Limit(pageSize, start).All(&goods)
	}

	beego.Info("sort ", sort, len(goods))
	this.Data["sort"] = sort

	this.Data["goodsType"] = goodsType
	this.Data["goods"] = goods
	this.Data["goodsNew"] = goodsNew

	this.TplName = "list.html"

}

/** 显示商品搜索页 */
func (this *GoodsController) ShowSearch() {
	// 默认获取全部商品
	o := orm.NewOrm()
	var goods []models.GoodsSKU
	o.QueryTable("GoodsSKU").All(&goods)
	// 返回视图
	this.Data["goods"] = goods
	ShowLayout(&this.Controller)
	this.TplName = "search.html"
}

/** 搜索商品处理 */
func (this *GoodsController) HandleSearch() {
	goodsName := this.GetString("goodsName")

	o := orm.NewOrm()
	var goods []models.GoodsSKU

	// 搜索内容为空，则搜索全部商品
	if goodsName == "" {
		o.QueryTable("GoodsSKU").All(&goods)
	} else {
		o.QueryTable("GoodsSKU").Filter("Name__icontains", goodsName).All(&goods)
	}

	this.Data["goods"] = goods
	ShowLayout(&this.Controller)
	this.TplName = "search.html"

}

/** 抽取的Layout方法，类型的用户名封装在内部 */
func ShowLayout(this *beego.Controller) {
	o := orm.NewOrm()
	var types []models.GoodsType
	// 获取类型
	o.QueryTable("GoodsType").All(&types)

	this.Data["types"] = types
	GetUser(this)
	GetCartCount(this)
	// 指定Layout
	this.Layout = "goods_layout.html"

}
