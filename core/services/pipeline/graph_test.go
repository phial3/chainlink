package pipeline

import (
	"net/url"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding/dot"

	"github.com/smartcontractkit/chainlink/core/store/models"
)

func TestGraph_Decode(t *testing.T) {
	expected := map[string]map[string]bool{
		"ds1": {
			"ds1":          false,
			"ds1_parse":    true,
			"ds1_multiply": false,
			"ds2":          false,
			"ds2_parse":    false,
			"ds2_multiply": false,
			"answer1":      false,
			"answer2":      false,
		},
		"ds1_parse": {
			"ds1":          false,
			"ds1_parse":    false,
			"ds1_multiply": true,
			"ds2":          false,
			"ds2_parse":    false,
			"ds2_multiply": false,
			"answer1":      false,
			"answer2":      false,
		},
		"ds1_multiply": {
			"ds1":          false,
			"ds1_parse":    false,
			"ds1_multiply": false,
			"ds2":          false,
			"ds2_parse":    false,
			"ds2_multiply": false,
			"answer1":      true,
			"answer2":      false,
		},
		"ds2": {
			"ds1":          false,
			"ds1_parse":    false,
			"ds1_multiply": false,
			"ds2":          false,
			"ds2_parse":    true,
			"ds2_multiply": false,
			"answer1":      false,
			"answer2":      false,
		},
		"ds2_parse": {
			"ds1":          false,
			"ds1_parse":    false,
			"ds1_multiply": false,
			"ds2":          false,
			"ds2_parse":    false,
			"ds2_multiply": true,
			"answer1":      false,
			"answer2":      false,
		},
		"ds2_multiply": {
			"ds1":          false,
			"ds1_parse":    false,
			"ds1_multiply": false,
			"ds2":          false,
			"ds2_parse":    false,
			"ds2_multiply": false,
			"answer1":      true,
			"answer2":      false,
		},
		"answer1": {
			"ds1":          false,
			"ds1_parse":    false,
			"ds1_multiply": false,
			"ds2":          false,
			"ds2_parse":    false,
			"ds2_multiply": false,
			"answer1":      false,
			"answer2":      false,
		},
		"answer2": {
			"ds1":          false,
			"ds1_parse":    false,
			"ds1_multiply": false,
			"ds2":          false,
			"ds2_parse":    false,
			"ds2_multiply": false,
			"answer1":      false,
			"answer2":      false,
		},
	}

	g := NewTaskDAG()
	err := g.UnmarshalText([]byte(DotStr))
	require.NoError(t, err)

	nodes := make(map[string]int64)
	iter := g.Nodes()
	for iter.Next() {
		n := iter.Node().(interface {
			graph.Node
			DOTID() string
		})
		nodes[n.DOTID()] = n.ID()
	}

	for from, connections := range expected {
		for to, connected := range connections {
			require.Equal(t, connected, g.HasEdgeFromTo(nodes[from], nodes[to]))
		}
	}
}

func TestGraph_TasksInDependencyOrder(t *testing.T) {
	g := NewTaskDAG()
	err := g.UnmarshalText([]byte(DotStr))
	require.NoError(t, err)

	u, err := url.Parse("https://chain.link/voter_turnout/USA-2020")
	require.NoError(t, err)

	answer1 := &MedianTask{
		BaseTask:      NewBaseTask("answer1", nil, 0),
		AllowedFaults: 1,
	}
	answer2 := &BridgeTask{
		Name:     "election_winner",
		BaseTask: NewBaseTask("answer2", nil, 1),
	}
	ds1_multiply := &MultiplyTask{
		Times:    decimal.NewFromFloat(1.23),
		BaseTask: NewBaseTask("ds1_multiply", answer1, 0),
	}
	ds1_parse := &JSONParseTask{
		Path:     []string{"one", "two"},
		BaseTask: NewBaseTask("ds1_parse", ds1_multiply, 0),
	}
	ds1 := &BridgeTask{
		Name:     "voter_turnout",
		BaseTask: NewBaseTask("ds1", ds1_parse, 0),
	}
	ds2_multiply := &MultiplyTask{
		Times:    decimal.NewFromFloat(4.56),
		BaseTask: NewBaseTask("ds2_multiply", answer1, 0),
	}
	ds2_parse := &JSONParseTask{
		Path:     []string{"three", "four"},
		BaseTask: NewBaseTask("ds2_parse", ds2_multiply, 0),
	}
	ds2 := &HTTPTask{
		URL:         models.WebURL(*u),
		Method:      "GET",
		RequestData: HttpRequestData{"hi": "hello"},
		BaseTask:    NewBaseTask("ds2", ds2_parse, 0),
	}

	tasks, err := g.TasksInDependencyOrder()
	require.NoError(t, err)

	// Make sure that no task appears in the array until its output task has already appeared
	for i, task := range tasks {
		if task.OutputTask() != nil {
			require.Contains(t, tasks[:i], task.OutputTask())
		}
	}

	expected := []Task{ds1, ds1_parse, ds1_multiply, ds2, ds2_parse, ds2_multiply, answer1, answer2}
	require.Len(t, tasks, len(expected))

	for _, task := range expected {
		require.Contains(t, tasks, task)
	}
}

func TestGraph_HasCycles(t *testing.T) {
	g := NewTaskDAG()
	err := g.UnmarshalText([]byte(DotStr))
	require.NoError(t, err)
	require.False(t, g.HasCycles())

	g = NewTaskDAG()
	err = dot.Unmarshal([]byte(`
        digraph {
            a [type=bridge];
            b [type=multiply times=1.23];
            a -> b -> a;
        }
    `), g)
	require.NoError(t, err)
	require.True(t, g.HasCycles())
}

func TestGraph_Digest(t *testing.T) {
	t.Run("Attribute order does not affect digest", func(t *testing.T) {
		g := NewTaskDAG()
		err := dot.Unmarshal([]byte(`
			digraph {
					a;
					b [a=1 b=2 c=3 d=4 e=5];
					a -> b;
			}
		`), g)
		require.NoError(t, err)

		digest, err := g.Digest()
		require.NoError(t, err)
		require.Equal(t, common.HexToHash("0x77c015ccaf370e85712cf4098f18b608dd7990629af90d7b5e9a7977322dc0c9"), digest)

		// change the order of the attrs in b node, failure if implemented improperly is non-deterministic
		g = NewTaskDAG()
		err = dot.Unmarshal([]byte(`
			digraph {
					a;
					b [e=5 d=4 c=3 b=2 a=1];
					a -> b;
			}
		`), g)
		require.NoError(t, err)

		digest2, err := g.Digest()
		require.NoError(t, err)
		require.Equal(t, digest, digest2)
	})

	t.Run("Graph order does not affect digest", func(t *testing.T) {
		g := NewTaskDAG()
		err := dot.Unmarshal([]byte(`
			digraph {
					a;
					b;
					c;
					d;
					e;
					a -> b;
					b -> c;
					c -> d;
					d -> e;
			}
		`), g)
		require.NoError(t, err)

		digest, err := g.Digest()
		require.NoError(t, err)
		require.Equal(t, common.HexToHash("0x4920da56e4f22bcd05c61ff469d2fd59f6f8c26fc4ca32b5bd8247bf24b2b051"), digest)

		// change the order of the graph declarations
		g = NewTaskDAG()
		err = dot.Unmarshal([]byte(`
			digraph {
					e;
					d;
					c;
					b;
					a;
					d -> e;
					c -> d;
					b -> c;
					a -> b;
			}
		`), g)
		require.NoError(t, err)

		digest2, err := g.Digest()
		require.NoError(t, err)
		require.Equal(t, digest, digest2)
	})
}
