package main

import (
	"archive/zip"
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/PuerkitoBio/goquery"
)

var fStructName = flag.String("s", "strucName", "specified struct name")
var ll = log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile)

func main() {

	flag.Parse()
	r, err := zip.OpenReader(flag.Arg(0))
	if err != nil {
		ll.Println(err)
		return
	}
	defer r.Close()

	var pack onePackage
	pack.PackageName = "test"
	pack.StructName = "structName"

	files := make(map[string]*zip.File)
	for _, f := range r.File {
		files[f.Name] = f // raw/02_s.txt
	}

	indexFile, exist := files["_index.htm"]
	if !exist {
		ll.Println("文件内容错误")
		return
	}

	read, err := indexFile.Open()
	if err != nil {
		ll.Println(err)
		return
	}

	doc, err := goquery.NewDocumentFromReader(read)
	if err != nil {
		ll.Println(err)
		return
	}

	//jsonStrs := [][]byte{}
	doc.Find("body table tbody tr").Each(func(i int, s *goquery.Selection) {

		reqName, ok0 := s.Find("td a").Eq(0).Attr("href")
		respName, ok1 := s.Find("td a").Eq(1).Attr("href")

		reqName = strings.Replace(reqName, "\\", "/", -1)
		respName = strings.Replace(respName, "\\", "/", -1)
		//ll.Println(reqName, respName)

		if ok0 && ok1 {
			reqFile, exist := files[reqName]
			if exist {
				reqRead, err := reqFile.Open()
				if err != nil {
					ll.Println(err)
					return
				}

				if method, err := parseRequest(i, bufio.NewReader(reqRead)); err != nil {
					ll.Println(err)
				} else {
					pack.Methods = append(pack.Methods, method)
				}

				reqRead.Close()
			}
		}
	})

	t := template.New("req")
	t, err = t.Parse(tmplPackage)
	if err != nil {
		ll.Println(err)
		return
	}

	if err := os.Mkdir("gen", 0755); err != nil {
		if !os.IsExist(err) {
			ll.Println(err)
			return
		}
	}

	//f, _ := os.Create("gen/gen.go")
	f, err := os.OpenFile("gen/gen.go", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		ll.Println(err)
		return
	}
	if err := t.Execute(f, pack); err != nil {
		ll.Println(err)
		return
	}

	f.Close()

	fmt.Println("生成成功 :)")
}

func parseRequest(count int, rc io.Reader) (m oneMethod, err error) {
	m.Heads = make(map[string]string)
	m.Params = make(map[string]string)
	m.RetryTimes = 3
	m.StructName = "structName"
	m.StructNameFirstChar = "s"

	s := bufio.NewScanner(rc)
	haveReadReqLine := false
	haveReadHeads := false
	isParamLine := false

	for s.Scan() {
		line := s.Text()
		if !haveReadReqLine {
			s1 := strings.Index(line, " ")
			s2 := strings.Index(line[s1+1:], " ")
			if s1 < 0 || s2 < 0 {
				ll.Println("解析请求头失败")
				err = errors.New("解析请求头失败")
				return
			}
			s2 += s1 + 1
			m.ReqMethod = strings.Title(strings.ToLower(line[:s1]))

			URI := line[s1+1 : s2]

			URL, err2 := url.Parse(URI)
			if err2 != nil {
				err = err2
				return
			}

			m.URL = URL.Scheme + "://" + URL.Host + URL.Path

			// 设置函数名
			index := strings.LastIndex(URL.Path, "/")
			if index < 0 {
				m.MethodMame = "defaultMethod" + strconv.Itoa(count)
			} else {
				lastStr := URL.Path[index+1:]
				if len(lastStr) == 0 {
					m.MethodMame = "defaultMethod" + strconv.Itoa(count)
				} else {
					m.MethodMame = lastStr
				}
			}

			// 解析参数
			params, err2 := url.ParseQuery(URL.RawQuery)
			if err2 != nil {
				err = err2

				ll.Println("解析参数失败")
				haveReadReqLine = true
				continue
			}

			for k := range params {
				m.Params[k] = params.Get(k)
			}

			haveReadReqLine = true
			continue
		}

		if len(line) > 0 {
			if !isParamLine {
				headSlice := strings.Split(line, ": ")
				if headSlice[0] != "Cookie" && headSlice[0] != "Content-Length" {
					m.Heads[headSlice[0]] = headSlice[1]
				}

				haveReadHeads = true
			} else {
				params, err2 := url.ParseQuery(line)
				if err2 != nil {
					err = err2

					ll.Println("解析参数失败")
					continue
				}

				for k := range params {
					m.Params[k] = params.Get(k)
				}
			}
		} else {
			if haveReadHeads && haveReadReqLine { // 下一行是参数行
				isParamLine = true
				continue
			}
		}
	}

	return
}

type onePackage struct {
	PackageName string
	StructName  string
	Methods     []oneMethod
}

type oneMethod struct {
	StructNameFirstChar string
	StructName          string
	MethodMame          string
	RetryTimes          int
	ReqMethod           string
	URL                 string
	Heads               map[string]string
	Params              map[string]string
}

var tmplPackage = `
package {{.PackageName}}

type {{.StructName}} struct {
	
}

{{range .Methods}}
func ({{.StructNameFirstChar}} *{{.StructName}}) {{.MethodMame}}() (resp string, err error) {
	
	for i := 0; i < conf.GSystemConfig.ReTryTimes; i++ {
		req := httpclient.{{.ReqMethod}}("{{.URL}}")
		{{range $key, $value :=  .Heads -}}
		req.Header("{{$key}}", "{{$value}}")
		{{end -}}
		{{if .Params}}
		{{range $key, $value :=  .Params -}}
		req.Param("{{$key}}", "{{$value}}")
		{{end -}}
		{{end}}
		req.SetCookieJar({{.StructNameFirstChar}}.ci.CICookieJar)

		if {{.StructNameFirstChar}}.ci.UseProxy {
			req.SetAuthProxy({{.StructNameFirstChar}}.ci.ProxyUser, {{.StructNameFirstChar}}.ci.ProxyPass, {{.StructNameFirstChar}}.ci.ProxyIp, {{.StructNameFirstChar}}.ci.ProxyPort)
		}

		resp, err = req.String()
		if err == nil {
			break
		}

		buslog.GSLog.Error({{.StructNameFirstChar}}.LogPrefix+"{{.MethodMame}}请求失败 resp:%s err:%s", resp, err.Error())

		utils.WaitRandMs(300, 500)
	}

	return 
}	
{{end}}
`
