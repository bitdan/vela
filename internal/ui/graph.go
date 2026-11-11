package ui

import (
	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type graphEdge struct{ top, bottom int }
type graphRow struct{ incoming, outgoing []graphEdge }

const lastGraphLane = 9

// buildGraph splits each parent edge at row boundaries. The resulting data
// can be painted independently by the virtualized log rows.
func buildGraph(history []domain.Commit) []graphRow {
	rows := make([]graphRow, len(history))
	deltas := make([][lastGraphLane*2 + 1]int, len(history))
	for child, entry := range history {
		childLane := min(entry.Lane, lastGraphLane)
		for _, parent := range entry.Parents {
			parentLane := min(history[parent].Lane, lastGraphLane)
			middle := childLane + parentLane
			rows[child].outgoing = append(rows[child].outgoing, graphEdge{childLane * 2, middle})
			if child+1 < parent {
				deltas[child+1][middle]++
				deltas[parent][middle]--
			}
			rows[parent].incoming = append(rows[parent].incoming, graphEdge{middle, parentLane * 2})
		}
	}
	var active [lastGraphLane*2 + 1]int
	for i, delta := range deltas {
		for lane, change := range delta {
			active[lane] += change
			if active[lane] > 0 {
				edge := graphEdge{lane, lane}
				rows[i].incoming = append(rows[i].incoming, edge)
				rows[i].outgoing = append(rows[i].outgoing, edge)
			}
		}
	}
	return rows
}

func drawCommit(p *ui.Painter, box ui.Rect, tokens tokens, entry domain.Commit, row graphRow) {
	x := func(lane int) float32 { return box.X + graphStart + float32(lane)*graphStep/2 }
	y := box.Y + box.H/2
	for _, edge := range row.incoming {
		p.Line(x(edge.top), box.Y, x(edge.bottom), y, graphStroke, tokens.graph[edge.bottom/2%len(tokens.graph)])
	}
	for _, edge := range row.outgoing {
		p.Line(x(edge.top), y, x(edge.bottom), box.Y+box.H, graphStroke, tokens.graph[edge.top/2%len(tokens.graph)])
	}
	radius := float32(graphDot)
	if entry.Merge {
		radius++
	}
	lane := min(entry.Lane, lastGraphLane)
	p.Fill(ui.Rect{X: x(lane*2) - radius, Y: y - radius, W: radius * 2, H: radius * 2}, tokens.graph[lane%len(tokens.graph)], radius)
}
