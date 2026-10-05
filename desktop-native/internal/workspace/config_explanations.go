package workspace

import (
	"fmt"
	"gioui.org/layout"
	"github.com/ffutop/modbus-gateway/internal/routing"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func normalizedIDs(expression string) string {
	if strings.TrimSpace(expression) == "" {
		return "默认路由：仅单个下游时接收全部未指定 ID 的请求"
	}
	ids, err := routing.ParseSlaveIDs(expression)
	if err != nil {
		return "ID 表达式无效：" + err.Error()
	}
	seen := map[int]bool{}
	values := []int{}
	for _, id := range ids {
		if !seen[int(id)] {
			seen[int(id)] = true
			values = append(values, int(id))
		}
	}
	sort.Ints(values)
	parts := []string{}
	for i := 0; i < len(values); {
		j := i
		for j+1 < len(values) && values[j+1] == values[j]+1 {
			j++
		}
		part := strconv.Itoa(values[i])
		if j > i {
			part += "–" + strconv.Itoa(values[j])
		}
		parts = append(parts, part)
		i = j + 1
	}
	return fmt.Sprintf("ID %s · 覆盖 %d 个（0–255）", strings.Join(parts, ", "), len(values))
}

func (e *configEditor) objectExplanation(gtx C, n *cfgNode) D {
	text := ""
	if d := e.downstream(n.path); d != nil {
		text = normalizedIDs(d.SlaveIDs)
	}
	if n.kind == "模拟模型" {
		cwd, _ := os.Getwd()
		for _, s := range e.draft.Simulations {
			if s.Name != n.title() {
				continue
			}
			switch s.Persistence.Type {
			case "", "memory":
				text = "memory：仅内存，整个进程重启后数据清空。"
			case "file", "mmap", "sql":
				resolved, err := filepath.Abs(s.Persistence.Path)
				if err != nil || strings.HasPrefix(s.Persistence.Path, "file:") || s.Persistence.Path == ":memory:" {
					resolved = s.Persistence.Path
				}
				text = fmt.Sprintf("%s：工作目录 %s；路径 %s。删除或改名模型不会删除数据文件。", s.Persistence.Type, cwd, resolved)
				if s.Persistence.Type == "sql" {
					text += "sql 使用 SQLite3；支持 SQLite 文件名或连接串，不提供服务器数据库驱动。"
				}
			}
			break
		}
	}
	if text == "" {
		return D{}
	}
	return layout.Inset{Left: 24, Right: 24, Bottom: 12}.Layout(gtx, e.th.label(text, smallSize, colMuted).Layout)
}
