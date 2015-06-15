package gui

import (
	"github.com/henrylee2cn/pholcus/config"
	"github.com/henrylee2cn/pholcus/pholcus/crawler"
	"github.com/henrylee2cn/pholcus/reporter"
	"github.com/henrylee2cn/pholcus/scheduler"
	"github.com/henrylee2cn/pholcus/spiders/spider"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"log"
	"strconv"
	"strings"
	"time"
)

var toggleSpecialModePB *walk.PushButton

func Run() {
	var mw *walk.MainWindow
	var db *walk.DataBinder
	var ep walk.ErrorPresenter

	if err := (MainWindow{
		AssignTo: &mw,
		DataBinder: DataBinder{
			AssignTo:       &db,
			DataSource:     Input,
			ErrorPresenter: ErrorPresenterRef{&ep},
		},
		Title:   config.APP_NAME,
		MinSize: Size{1100, 700},
		Layout:  VBox{},
		Children: []Widget{
			// 任务列表
			HSplitter{
				Children: []Widget{
					TableView{
						MinSize:               Size{550, 400},
						AlternatingRowBGColor: walk.RGB(255, 255, 224),
						CheckBoxes:            true,
						ColumnsOrderable:      true,
						Columns: []TableViewColumn{
							{Title: "#", Width: 45},
							{Title: "任务", Width: 110 /*, Format: "%.2f", Alignment: AlignFar*/},
							{Title: "描述", Width: 370},
						},
						Model: SpiderModel,
					},
					// 关键词
					VSplitter{
						MinSize: Size{550, 400},

						Children: []Widget{
							VSplitter{
								Children: []Widget{
									Label{
										Text: "关键词：（多任务之间以 | 隔开，选填）",
									},
									LineEdit{
										Text: Bind("Keywords"),
									},
								},
							},

							VSplitter{
								Children: []Widget{
									Label{
										Text: "采集页数：（选填）",
									},
									NumberEdit{
										Value:    Bind("MaxPage"),
										Suffix:   "",
										Decimals: 0,
									},
								},
							},

							VSplitter{
								Children: []Widget{
									Label{
										Text: "*并发协程：（1~99999）",
									},
									NumberEdit{
										Value:    Bind("ThreadNum", Range{1, 99999}),
										Suffix:   "",
										Decimals: 0,
									},
								},
							},

							VSplitter{
								Children: []Widget{
									Label{
										Text: "*分批输出大小：（1~5,000,000 条数据）",
									},
									NumberEdit{
										Value:    Bind("DockerCap", Range{1, 5000000}),
										Suffix:   "",
										Decimals: 0,
									},
								},
							},

							VSplitter{
								Children: []Widget{
									Label{
										Text: "*间隔基准:",
									},
									ComboBox{
										Value:         Bind("BaseSleeptime", SelRequired{}),
										BindingMember: "Uint",
										DisplayMember: "Key",
										Model:         GUIOpt.SleepTime,
									},
								},
							},

							VSplitter{
								Children: []Widget{
									Label{
										Text: "*随机延迟:",
									},
									ComboBox{
										Value:         Bind("RandomSleepPeriod", SelRequired{}),
										BindingMember: "Uint",
										DisplayMember: "Key",
										Model:         GUIOpt.SleepTime,
									},
								},
							},

							RadioButtonGroupBox{
								ColumnSpan: 2,
								Title:      "*输出方式",
								Layout:     HBox{},
								DataMember: "OutType",
								Buttons: []RadioButton{
									{Text: GUIOpt.OutType[0].Key, Value: GUIOpt.OutType[0].String},
									{Text: GUIOpt.OutType[1].Key, Value: GUIOpt.OutType[1].String},
									{Text: GUIOpt.OutType[2].Key, Value: GUIOpt.OutType[2].String},
								},
							},
						},
					},
				},
			},

			Composite{
				Layout: HBox{},
				Children: []Widget{

					// 必填项错误检查
					LineErrorPresenter{
						AssignTo:   &ep,
						ColumnSpan: 2,
					},

					PushButton{
						Text:     "开始抓取",
						AssignTo: &toggleSpecialModePB,
						OnClicked: func() {
							if toggleSpecialModePB.Text() == "取消" {
								toggleSpecialModePB.SetEnabled(false)
								toggleSpecialModePB.SetText("取消中…")
								Stop()
							} else {
								if err := db.Submit(); err != nil {
									log.Print(err)
									return
								}
								Input.Spiders = SpiderModel.GetChecked()
								if len(Input.Spiders) == 0 {
									return
								}
								toggleSpecialModePB.SetText("取消")
								Start()
							}
						},
					},
				},
			},
		},
	}.Create()); err != nil {
		log.Fatal(err)
	}

	// 绑定log输出界面
	lv, err := NewLogView(mw)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(lv)

	if icon, err := walk.NewIconFromResource("ICON"); err == nil {
		mw.SetIcon(icon)
	}

	// 运行窗体程序
	mw.Run()
}

var status int

const (
	STOP = iota
	RUN
)

// 提交用户输入并开始运行
func Start() {
	// 初始化蜘蛛列表，返回长度
	count := InitSpiders()

	// 初始化config参数
	config.InitDockerParam(Input.DockerCap)

	if Input.ThreadNum == 0 {
		// 纠正协程数
		Input.ThreadNum = 1
	}
	config.ThreadNum = Input.ThreadNum
	config.OutType = Input.OutType
	config.StartTime = time.Now()
	config.ReqSum = 0

	// 初始化资源队列
	scheduler.Init(Input.ThreadNum)

	// 初始化爬虫队列
	CrawlerNum := config.CRAWLER_CAP
	if count < config.CRAWLER_CAP {
		CrawlerNum = count
	}
	crawler.CQ.Init(uint(CrawlerNum))

	// 开启报告
	reporter.Log.Run()
	reporter.Log.Printf("\n执行任务总数（任务数[*关键词数]）为 %v 个...\n", count)
	reporter.Log.Printf("\n爬虫队列可容纳蜘蛛 %v 只...\n", CrawlerNum)
	reporter.Log.Printf("\n并发协程最多 %v 个……\n", Input.ThreadNum)
	reporter.Log.Printf("\n随机停顿时间为 %v~%v ms ……\n", Input.BaseSleeptime, Input.BaseSleeptime+Input.RandomSleepPeriod)
	reporter.Log.Printf("*********************************************开始抓取，请耐心等候*********************************************")

	// 任务执行
	status = RUN
	go GoRun(count)
}

// 任务执行
func GoRun(count int) {
	for i := 0; i < count && status == RUN; i++ {
		// 从爬行队列取出空闲蜘蛛，并发执行
		c := crawler.CQ.Use()

		if c != nil {
			go func(i int, c crawler.Crawler) {
				// 执行并返回结果消息
				c.Init(spider.SpiderList[i]).Start()
				// 任务结束后回收该蜘蛛
				crawler.CQ.Free(c.GetId())
			}(i, c)
		}
	}

	// 监控结束任务
	sum := 0 //数据总数
	for i := 0; i < count; i++ {
		s := <-config.ReportChan
		reporter.Log.Printf("[结束报告 -> 任务：%v | 关键词：%v] 共输出数据 %v 条，用时 %v 分钟！！！\n", s.SpiderName, s.Keyword, s.Num, s.Time)
		if slen, err := strconv.Atoi(s.Num); err == nil {
			sum += slen
		}
	}
	reporter.Log.Printf("*****************************！！本次抓取合计 %v 条数据，下载页面 %v 个，耗时：%.5f 分钟！！***************************", sum, config.ReqSum, time.Since(config.StartTime).Minutes())

	// 按钮状态控制
	toggleSpecialModePB.SetEnabled(true)
	toggleSpecialModePB.SetText("开始抓取")

}

//中途终止任务
func Stop() {
	status = STOP
	crawler.CQ.Stop()
	scheduler.Sdl.Stop()
	reporter.Log.Stop()
	log.Printf("************************！！任务取消：下载页面 %v 个，耗时：%.5f 分钟！！**********************", config.ReqSum, time.Since(config.StartTime).Minutes())
	// 按钮状态控制
	toggleSpecialModePB.SetEnabled(true)
	toggleSpecialModePB.SetText("开始抓取")
}

// 用户提交后，生成蜘蛛列表
func InitSpiders() int {
	var sp = spider.Spiders{}
	spider.SpiderList.Init()

	// 遍历任务
	for i, sps := range Input.Spiders {
		sp = append(sp, sps.Spider)
		l := len(sp) - 1
		sp[l].Id = i
		sp[l].Pausetime[0] = Input.BaseSleeptime
		sp[l].Pausetime[1] = Input.RandomSleepPeriod
		sp[l].MaxPage = Input.MaxPage
	}

	// 遍历关键词
	if Input.Keywords != "" {
		keywordSlice := strings.Split(Input.Keywords, "|")
		for _, keyword := range keywordSlice {
			keyword = strings.Trim(keyword, " ")
			if keyword == "" {
				continue
			}
			nowLen := len(spider.SpiderList)
			for n, _ := range sp {
				sp[n].Keyword = keyword
				sp[n].Id = nowLen + n
				c := *sp[n]
				spider.SpiderList.Add(&c)
			}
		}
	} else {
		spider.SpiderList = sp
	}
	return len(spider.SpiderList)
}
