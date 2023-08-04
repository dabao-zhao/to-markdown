package main

import (
	"log"
	netUrl "net/url"
	"os"
	xpath "path"
	"regexp"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	toMd "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	xhttp "github.com/dabao-zhao/helper/http"
	xmd5 "github.com/dabao-zhao/helper/md5"
)

var (
	getUrlContentBtn *widget.Button
	infProgress      *widget.ProgressBarInfinite
)

func main() {
	registerChinese()

	a := app.New()
	a.Settings().SetTheme(theme.LightTheme())

	w := a.NewWindow("to-markdown")
	w.Resize(fyne.NewSize(800, 600))

	urlInput := widget.NewMultiLineEntry()
	urlInput.SetPlaceHolder("输入文章地址，多个地址使用回车区分")
	urlInput.SetMinRowsVisible(10)

	infProgress = widget.NewProgressBarInfinite()
	infProgress.Hide()

	getUrlContentBtn = widget.NewButton("获取", func() {
		getUrlContentBtn.SetText("运行中……")
		infProgress.Show()
		defer func() {
			getUrlContentBtn.SetText("获取")
			infProgress.Hide()
		}()
		text := urlInput.Text
		if text == "" {
			return
		}
		urls := strings.Split(text, "\n")
		for _, url := range urls {
			if url == "" {
				continue
			}
			toMarkdown(url)
		}
	})
	getUrlContentBtn.Importance = widget.HighImportance

	c := container.NewVBox(urlInput, getUrlContentBtn, infProgress)

	w.SetContent(c)

	w.Show()

	a.Run()
}

func registerChinese() {
	_ = os.Setenv("FYNE_FONT", "./font/simkai.ttf")
}

func toMarkdown(url string) {
	html := getHtml(url)
	if html == "" {
		return
	}
	title := getHtmlTitle(html)
	path := mkDir(title)
	content := getHtmlContent(url, html)
	if content == "" {
		return
	}
	md := htmlToMarkdown(content)
	if md == "" {
		return
	}
	md = replaceImg(path, md, url)
	if md == "" {
		return
	}
	saveMd(path, title, md)
}

// 获取 url 的 html
func getHtml(url string) string {
	u, err := netUrl.Parse(url)
	if err != nil {
		log.Println(err)
		return ""
	}
	client := xhttp.NewHttp(xhttp.WithTimeout(time.Second*5), xhttp.WithHeaders(map[string]string{
		"Content-Type": "text/html; charset=utf-8",
		"User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36",
		"Referer":      u.Scheme + "://" + u.Host,
	}))
	data, err := client.Get(url, nil)
	if err != nil {
		log.Println(err)
		return ""
	}
	return string(data)
}

// 处理 html，获取文章标题
func getHtmlTitle(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return time.Now().Format("20060102150405")
	}
	return doc.Find("html > head > title").Text()
}

// 处理 html，获取文章内容
func getHtmlContent(url, html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	if strings.Contains(url, "juejin") {
		h, _ := doc.Find(".article").First().Html()
		return h
	}
	if strings.Contains(url, "jianshu") {
		h, _ := doc.Find(".ouvJEz").First().Html()
		return h
	}
	if strings.Contains(url, "zhihu") {
		// 目前无法爬取，触发安全验证
		h, _ := doc.Find(".RichText").First().Html()
		return h
	}
	if strings.Contains(url, "csdn") {
		h, _ := doc.Find(".blog-content-box").First().Html()
		return h
	}
	if strings.Contains(url, "oschina") {
		h, _ := doc.Find(".article-detail").First().Html()
		return h
	}
	if strings.Contains(url, "cnblogs") {
		h, _ := doc.Find("#cnblogs_post_body").First().Html()
		return h
	}

	h, _ := doc.Find("html > body").Html()
	return h
}

// 将 html 转为 markdown
func htmlToMarkdown(html string) string {
	converter := toMd.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(html)
	if err != nil {
		log.Println(err)
		return ""
	}
	return markdown
}

func replaceImg(path, toMd, referer string) string {
	// 过滤所有图片
	imgPattern := "\\!\\[.*?\\]\\((.*?)\\)"
	imgReg, _ := regexp.Compile(imgPattern)
	imgMds := imgReg.FindAll([]byte(toMd), -1)
	urlPattern := "\\((.*?)\\)"
	urlReg, _ := regexp.Compile(urlPattern)
	for _, b := range imgMds {
		res := urlReg.Find(b)
		if len(res) == 0 {
			continue
		}
		url := string(res)[1 : len(string(res))-1]
		if url == "" {
			continue
		}
		newUrl := saveImage(path, url, referer)
		toMd = strings.Replace(toMd, url, newUrl, -1)
	}
	return toMd
}

// 处理 markdown 内的图片，进行转存然后返回新的路径
func saveImage(path, img, referer string) string {
	client := xhttp.NewHttp(xhttp.WithTimeout(time.Second*5), xhttp.WithHeaders(map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36",
		"Referer":    referer,
	}))
	data, err := client.Get(img, nil)
	if err != nil {
		log.Println(err)
		return img
	}
	ext := imgExt(img)
	imgUlrMd5, _ := xmd5.String(img)
	filename := imgUlrMd5 + ext
	filePath := path + "/" + filename
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Println(err)
		return img
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		log.Println(err)
		return img
	}

	return filename
}

func saveMd(path, title, md string) {
	filePath := path + "/" + title + ".md"
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()
	_, err = f.WriteString(md)
	if err != nil {
		log.Println(err)
		return
	}
}

func mkDir(title string) string {
	path := "posts/" + title
	_ = os.MkdirAll(path, 0777)
	return path
}

func imgExt(img string) string {
	ext := xpath.Ext(img)
	for i, r := range ext {
		if i == 0 {
			continue
		}
		if unicode.IsLetter(r) {
			continue
		}
		return ext[:i]
	}
	return ext
}
