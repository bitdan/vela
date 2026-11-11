package ui

import (
	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type graphEdge struct{ top, bottom int }
type graphRow struct{ incoming, outgoing []graphEdge }

// buildGraph splits each parent edge at row boundaries. The resulting data
// can be painted independently by the virtualized log rows.
func buildGraph(history []domain.Commit) []graphRow {
	rows := make([]graphRow, len(history))
	for child, entry := range history {
		for _, parent := range entry.Parents {
			parentLane := history[parent].Lane
			rows[child].outgoing = append(rows[child].outgoing, graphEdge{entry.Lane * 2, entry.Lane + parentLane})
			for i := child + 1; i < parent; i++ {
				lane := entry.Lane + parentLane
				rows[i].incoming = append(rows[i].incoming, graphEdge{lane, lane})
				rows[i].outgoing = append(rows[i].outgoing, graphEdge{lane, lane})
			}
			rows[parent].incoming = append(rows[parent].incoming, graphEdge{entry.Lane + parentLane, parentLane * 2})
		}
	}
	return rows
}

func drawCommit(p *ui.Painter, box ui.Rect, tokens tokens, entry domain.Commit, row graphRow) {
	x := func(lane int) float32 { return box.X + graphStart + float32(lane)*graphStep/2 }
	y := box.Y + box.H/2
	for _, edge := range row.incoming {
		p.Line(x(edge.top), box.Y, x(edge.bottom), y, graphStroke, tokens.graph[edge.bottom/2])
	}
	for _, edge := range row.outgoing {
		p.Line(x(edge.top), y, x(edge.bottom), box.Y+box.H, graphStroke, tokens.graph[edge.top/2])
	}
	radius := float32(graphDot)
	if entry.Merge {
		radius++
	}
	p.Fill(ui.Rect{X: x(entry.Lane*2) - radius, Y: y - radius, W: radius * 2, H: radius * 2}, tokens.graph[entry.Lane], radius)
}
