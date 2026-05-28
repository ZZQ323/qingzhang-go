// Package bill 解析微信/支付宝导出的账单 CSV，转成可入库的记录。
package bill

import (
	"bytes"
	"encoding/csv"
	"io"
	"math"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"qingzhang/internal/apperr"
	"qingzhang/internal/db"
)

const maxExtIDLen = 128 // external_id 上限，防脏数据撑爆

// 各来源的列名差异
type colSpec struct {
	time, direction, amount, extID, counterparty, note, category string
}

var specs = map[string]colSpec{
	"wx": {
		time: "交易时间", direction: "收/支", amount: "金额(元)", extID: "交易单号",
		counterparty: "交易对方", note: "商品", category: "交易类型",
	},
	"alipay": {
		time: "交易时间", direction: "收/支", amount: "金额", extID: "交易订单号",
		counterparty: "交易对方", note: "商品说明", category: "交易分类",
	},
}

// Parse 解析账单。source 为 "wx" 或 "alipay"，raw 为上传的 CSV 原始字节。
func Parse(source string, raw []byte) ([]db.Record, error) {
	spec, ok := specs[source]
	if !ok {
		return nil, apperr.Param("不支持的账单来源：" + source)
	}

	// 支付宝为 GBK，微信为 UTF-8（可能带 BOM）
	text := raw
	if source == "alipay" {
		dec, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), raw)
		if err != nil {
			return nil, apperr.Param("账单编码解析失败（应为支付宝 GBK 文件）")
		}
		text = dec
	}
	text = bytes.TrimPrefix(text, []byte{0xEF, 0xBB, 0xBF}) // 去 UTF-8 BOM

	r := csv.NewReader(bytes.NewReader(text))
	r.FieldsPerRecord = -1 // 说明行字段数不一，允许变长

	var headerIdx map[string]int
	var out []db.Record
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 个别坏行跳过，不让整份失败
			continue
		}
		// 找表头行：含「交易时间」与「收/支」
		if headerIdx == nil {
			if containsField(row, spec.time) && containsField(row, spec.direction) {
				headerIdx = indexHeader(row)
			}
			continue
		}
		rec, ok := parseRow(row, headerIdx, spec)
		if ok {
			out = append(out, rec)
		}
	}
	if headerIdx == nil {
		return nil, apperr.Param("未识别到账单表头，请确认文件来源与是否已解压为 CSV")
	}
	return out, nil
}

func parseRow(row []string, idx map[string]int, spec colSpec) (db.Record, bool) {
	get := func(name string) string {
		i, ok := idx[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	// 方向：仅收入/支出入账，中性交易/不计收支跳过
	var typ int
	switch get(spec.direction) {
	case "支出":
		typ = 1
	case "收入":
		typ = 2
	default:
		return db.Record{}, false
	}

	fen := parseAmount(get(spec.amount))
	if fen <= 0 {
		return db.Record{}, false
	}

	// 单号清洗：去所有空白，超长跳过
	extID := strings.Join(strings.Fields(get(spec.extID)), "")
	if len(extID) > maxExtIDLen {
		return db.Record{}, false
	}

	day := get(spec.time)
	if len(day) >= 10 {
		day = day[:10]
	}

	return db.Record{
		Type:         typ,
		Amount:       fen,
		Category:     mapCategory(get(spec.category), get(spec.note), get(spec.counterparty)),
		Note:         get(spec.note),
		Counterparty: get(spec.counterparty),
		HappenedAt:   day,
		ExternalID:   extID,
	}, true
}

// parseAmount 把 "¥16.50" / "1,234.00" 等转成分
func parseAmount(s string) int64 {
	s = strings.NewReplacer("¥", "", "￥", "", ",", "", " ", "").Replace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f * 100))
}

func containsField(row []string, name string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) == name {
			return true
		}
	}
	return false
}

func indexHeader(row []string) map[string]int {
	m := make(map[string]int, len(row))
	for i, c := range row {
		m[strings.TrimSpace(c)] = i
	}
	return m
}
