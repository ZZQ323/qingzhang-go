package bill

import "strings"

// 账单原始分类/类型/商品说明 -> 我们的类别。匹配不到归「其他」。
// 关键词命中即可，顺序不敏感（取第一个命中）。
var keywordCategory = []struct {
	keys []string
	cat  string
}{
	{[]string{"餐饮", "美食", "外卖", "饿了么", "美团", "肯德基", "麦当劳", "星巴克", "饭", "食"}, "餐饮"},
	{[]string{"交通", "打车", "滴滴", "地铁", "公交", "高铁", "火车", "出行", "加油", "停车"}, "交通"},
	{[]string{"购物", "淘宝", "天猫", "京东", "拼多多", "超市", "百货", "服饰", "日用"}, "购物"},
	{[]string{"居家", "房租", "物业", "水电", "燃气", "家居"}, "居家"},
	{[]string{"娱乐", "电影", "游戏", "视频", "音乐", "休闲", "文化"}, "娱乐"},
	{[]string{"医疗", "医院", "药", "诊所", "健康"}, "医疗"},
	{[]string{"学习", "教育", "书", "课程", "培训", "文具"}, "学习"},
	{[]string{"红包", "转账", "人情", "礼"}, "人情"},
	{[]string{"工资", "薪"}, "工资"},
	{[]string{"退款"}, "退款"},
}

// mapCategory 综合账单的分类、交易类型、商品说明做关键词匹配
func mapCategory(parts ...string) string {
	hay := strings.Join(parts, " ")
	for _, kc := range keywordCategory {
		for _, k := range kc.keys {
			if strings.Contains(hay, k) {
				return kc.cat
			}
		}
	}
	return "其他"
}
