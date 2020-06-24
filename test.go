package main

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"
)

func main() {
	//engine := gin.Default()
	//engine.GET("/ping", func(ctx *gin.Context) {
	//	ctx.JSON(200, gin.H{
	//		"message": "pong",
	//	})
	//})
	//engine.Run()	// 监听 0.0.0.0:8080

	user := &UserInfo{
		Name: "小明",
		Age:  23,
		Courses: []string{"语文", "数学", "英语"},
		AliasA: Attr{
			"A",
		},
		AliasB: &Attr{
			"B",
		},
		Neighbors: []*UserInfo{
			&UserInfo{
				Name:      "QQ",
				Age:       10,
				Courses:   nil,
				AliasA:    Attr{},
				AliasB:    nil,
			},
			&UserInfo{
				Name:      "PP",
				Age:       20,
				Courses:   nil,
				AliasA:    Attr{},
				AliasB:    nil,
			},
		},
		XXX_abc: "xxx",
		PB: proto.String("PB"),
	}

	msg, _ := ParseAnyStructPtrIntoMsg(user, "  ", 3, "XXX_*")
	RspWithMsg(msg)
}


type UserInfo struct {
	Name string
	Age int
	Courses []string
	AliasA Attr
	AliasB *Attr
	Neighbors []*UserInfo
	XXX_abc string
	PB *string
}

type Attr struct {
	Alias string
}

// 将结构体中字段与值读为Pair数组，并且根据递归深度增加缩进量
// val必须是结构体类型或者结构体指针类型，否则触发panic并recover
// filter为过滤函数，可以对满足某正则表达式的字段名进行过滤(忽略)
func parsePairRecursive(val reflect.Value, pairs *[]*Pair, depth int, maxDepth int, filter func(filedName string) bool) {
	// 由于没法预料到可能出现的所有意外，所以加一个recover，即便意料之外的panic，也不会导致程序中断，仅仅是解析失败
	defer func() {
		if err := recover(); err != nil {
			// 打日志
			return
		}
	}()

	// val如果是Ptr则将val转为实际类型
	val = checkValueAndTurnPtrToElem(val)

	// 为避免调用栈过深，同时也考虑展示效果，在达到最大深度时则无论如何都停止递归
	// 这段是可用可无的，因为后续同样有逻辑处理递归截止问题
	//if depth >= maxdepth {
	//	*pairs = append(*pairs, &Pair{
	//		K: val.Type().Name(),
	//		V: fmt.Sprintf("%v", val.Interface()),
	//		Depth: depth,
	//	})
	//	return
	//}

	// 正常递归
	fieldNum := val.Type().NumField()	// 获取字段数.
	for i:= 0;i<fieldNum;i++ {
		// 过滤字段名。如果匹配到正则表达式，则忽略该字段。比如忽略"XXX_xxxx"的字段名
		if filter(val.Type().Field(i).Name) {
			continue
		}

		valI := val.Field(i)
		valI = checkValueAndTurnPtrToElem(valI)

		// 1. valI如果是结构体则递归处理，并且未达到最大递归深度
		if valI.Kind() == reflect.Struct && depth < maxDepth {
			*pairs = append(*pairs, &Pair{
				K: val.Type().Field(i).Name,
				V: "",	// 空内容，具体内容换行展示
				Depth: depth,
			})
			parsePairRecursive(valI, pairs, depth + 1, maxDepth, filter)
			continue
		}	// 是结构体但是已达最大深度，则将到达第3步

		// 2. valI是结构体数组或者结构体数组，则检查下元素是否是结构体，不是则直接展示，是则递归
		if valI.Kind() == reflect.Slice || valI.Kind() == reflect.Array {
			if valI.Len() > 0 {		// 数组长度至少为1时检查内部元素
				firstElem := valI.Index(0)
				firstElem = checkValueAndTurnPtrToElem(firstElem)
				if firstElem.Kind() == reflect.Struct && depth < maxDepth {		// 说明这是个结构体数组，并且未达到最大递归深度
					arrayName := val.Type().Field(i).Name	// 数组或slice名
					// 先将当前字段名生成Pair
					*pairs = append(*pairs, &Pair{
						K: arrayName,
						V: "[...]",	// 表示接下来缩进的是数组内容
						Depth: depth,
					})
					// 递归
					for i:=0; i<valI.Len();i++ {
						//fmt.Println("123", valI.Index(i).Elem().Kind(), valI.Index(i), depth+1)
						*pairs = append(*pairs, &Pair{
							K: fmt.Sprintf("%s[%d]", arrayName, i),
							V: "",
							Depth: depth+1,
						})
						parsePairRecursive(valI.Index(i), pairs, depth+2, maxDepth, filter)
					}
					continue
				}
				// 如果数组元素不是结构体，则程序将执行到第3步
			}
			// 如果数组长度为0，则程序将执行到第3步
		}

		// 3. 不是结构体则直接解码成Pair
		*pairs = append(*pairs, &Pair{
			K: val.Type().Field(i).Name,
			V: fmt.Sprintf("%v", checkValueAndTurnPtrToElem(val.Field(i)).Interface()),
			Depth: depth,
		})
	}
}

func checkValueAndTurnPtrToElem(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v
}

// ParseAnyStructPtrIntoMsg 将任意结构体或结构体指针传入，将之解析入Msg结构，用以展示
// NOTE: 虽然可以递归处理，但是不建议嵌套层数过多的结构体直接传入，手机端展示效果受限
// NOTE: 目前的实现是只有当成员为结构体、结构体指针、结构体数组、结构体指针数组才会递归解析，而成员为map等其他聚合类结构，并不进行递归解析处理，考量是：没必要进行过分的解析，提供目前常见的一些支持就行了
func ParseAnyStructPtrIntoMsg(structPtr interface{}, indent string, maxDepth int, filterRegexps ...string) (*Msg, error) {
	// 填充msg参数
	if maxDepth < 1 {	// maxDepth至少应为1，如果没填或者填的负数，那么治理将他修改为3
		maxDepth = 3
	}
	msg := &Msg{
		Indent: indent,
		MaxDepth: maxDepth,
		FilterRegexps: filterRegexps,
	}

	// 预处理正则表达式
	filters := make([]*regexp.Regexp, len(filterRegexps))
	for i, f := range filterRegexps {
		filters[i] = regexp.MustCompile(f)
	}
	// 定义过滤函数
	filterFunc := func(fieldName string) bool {
		for _, filter := range filters {
			if filter.MatchString(fieldName) {
				return true
			}
		}
		return false
	}

	// 检查Value，必须是或者指向结构体
	val := reflect.ValueOf(structPtr)
	val = checkValueAndTurnPtrToElem(val)	// 如果是指针，则获取其指向的内容
	if val.Kind() != reflect.Struct {
		return nil, errors.New("require struct pointer or struct")
	}

	// 填充msg.Head为结构体类型名
	typStrs := strings.Split(val.Type().String(), ".")
	msg.Head = typStrs[len(typStrs)-1]

	// 构建pairs，递归解析结构体字段到pairs中
	pairs := new([]*Pair)	// 需用*[]*Pair传入递归函数，因为如果发生扩容，则函数内外的切片不再是同一个，需用指针来保证始终能访问到最新的slice
	parsePairRecursive(val, pairs, 0, maxDepth, filterFunc)

	// 填充msg.Body为Pairs
	msg.Body = *pairs

	return msg, nil
}

type Pair struct {
	K string
	V string
	Depth int	// Depth其实就是递归深度，(Depth=0则需要加粗处理，否则不进行加粗展示)(这部分描述目前没有做); 此外Depth用于控制缩进程度
}

// Msg 定义为一条完整的有标题，有主体的消息.
// Body中每个元素代表一个键值对，为了保证有序性，使用[][2]string作为body而非map[string][string]
// 为了有更强的提示性，使用Pair表示键值对，替代[2]string
type Msg struct {
	Head string
	Body []*Pair

	// Indent为缩进字符，通常可设置为"  "(两个空格)或者"=="等，但注意不要设置"-"，"#"，"*"等，可能与markdown语法冲突。 实际缩进效果为 Pair.Level * indent
	Indent string	// 结构体内存在嵌套情形下使用缩进展示(例如"  xxx")，置空时则不缩进
	MaxDepth int	// 最大允许的递归深度，为了避免调用栈过深，也为了展示效果考虑，MaxDepth建议设置为3

	// FilterRegexps 过滤字段名用的正则表达式（若干个），满足正则表达式的字段名将被忽略，请谨慎设置。常用的做法是使用"XXX_*"忽略proto自动生成的字段
	FilterRegexps []string
}

func (msg *Msg) String() string {
	var result []string
	result = append(result,
		//"<font color=\"info\">Head</font>")
		MarkdownInfoText(msg.Head))
	for _, line := range msg.Body {		// Body为nil时跳过
		result = append(result,
			//fmt.Sprintf("[indent] **K**: <font color=\"comment\">%s</font>", V))
			strings.Repeat(msg.Indent, line.Depth) + MarkdownBoldNameCommentText(line.K, line.V))
	}

	msgStr := strings.Join(result, "\n")
	return msgStr
}

// 常用的返回结果回应方法
func RspWithMsg(msg *Msg) {
	msgStr := msg.String()
	fmt.Println(msgStr)
	file, err := os.OpenFile("./test.md", os.O_CREATE | os.O_RDWR, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	err = ioutil.WriteFile(file.Name(), []byte(msgStr), os.ModePerm)
	if err != nil {
		panic(err)
	}
}

func MarkdownInfoText(text string) string {
	return fmt.Sprintf("<font color=\"info\">%s</font>", text)
}
// MarkdownPairText markdown form style text, 'bold_name: comment_text'
func MarkdownBoldNameCommentText(name, text string) string {
	return fmt.Sprintf("**%s**: <font color=\"comment\">%v</font>", name, text)
}