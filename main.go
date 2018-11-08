package main

import (
	_ "DailyFresh/routers"
	"github.com/astaxie/beego"
	_ "DailyFresh/models"
)

func main() {
	beego.Run()
}

