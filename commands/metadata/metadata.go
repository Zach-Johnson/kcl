// Package metadata provides the metadata command.
package metadata

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/kcl/client"
	"github.com/twmb/kcl/out"
)

func Command(cl *client.Client) *cobra.Command {
	req := kmsg.MetadataRequest{}

	var pcluster, pbrokers, ptopics, pinternal, pall, detailed bool
	var ids bool

	cmd := &cobra.Command{
		Use:   "metadata [TOPICS]",
		Short: "Issue a metadata command and dump the results",
		Long: `Request metadata (0.8.0+).

Kafka's metadata contains a good deal of information about brokers, topics,
and the cluster as a whole. This is the command to use to get general info
on the what of everything.

To avoid noise, this command only prints requested sections. Additionally,
since there is a lot of information in topics, this prints short information
for topics unless detailed is requested. It is optional to specify which
topics to list metadata for; by default, all topics are listed.

If the brokers section is printed, the controller broker is marked with *.
`,

		Run: func(_ *cobra.Command, topics []string) {
			if len(topics) > 0 {
				ptopics = true
			}
			sections := 0
			for _, v := range []*bool{&pcluster, &pbrokers, &ptopics} {
				if pall {
					*v = true
				}
				if *v {
					sections++
				}
			}
			if sections == 0 && !cl.AsJSON() {
				out.Die("no metadata section requested")
			}

			includeHeader := sections > 1

			if !ptopics {
				req.Topics = []kmsg.MetadataRequestTopic{} // nil is all, empty is none
			} else {
				for _, topic := range topics {
					t := kmsg.NewMetadataRequestTopic()
					if ids {
						if len(topic) != 32 {
							out.Die("topic id %s is not a 32 byte hex string")
						}
						raw, err := hex.DecodeString(topic)
						out.MaybeDie(err, "topic id %s is not a hex string")
						copy(t.TopicID[:], raw)
					} else {
						t.Topic = kmsg.StringPtr(topic)
					}
					req.Topics = append(req.Topics, t)
				}
			}

			kresp, err := cl.Client().Request(context.Background(), &req)
			out.MaybeDie(err, "unable to get metadata: %v", err)
			if cl.AsJSON() {
				out.ExitJSON(kresp)
			}
			resp := kresp.(*kmsg.MetadataResponse)

			if pcluster && resp.ClusterID != nil {
				if includeHeader {
					fmt.Printf("CLUSTER\n=======\n")
				}
				fmt.Printf("%s\n", *resp.ClusterID)
				if includeHeader {
					fmt.Println()
				}
			}

			if pbrokers {
				if includeHeader {
					fmt.Printf("BROKERS\n=======\n")
				}
				printBrokers(resp.ControllerID, resp.Brokers)
				if includeHeader {
					fmt.Println()
				}
			}

			if ptopics && len(resp.Topics) > 0 {
				if includeHeader {
					fmt.Printf("TOPICS\n======\n")
				}
				printTopics(resp.Version, resp.Topics, pinternal, detailed)
			}
		},
	}

	cmd.Flags().BoolVarP(&pcluster, "cluster", "c", false, "print cluster section")
	cmd.Flags().BoolVarP(&pbrokers, "brokers", "b", false, "print brokers section")
	cmd.Flags().BoolVarP(&ptopics, "topics", "t", false, "print topics section (this flag is implied if any topics are input)")
	cmd.Flags().BoolVar(&ids, "ids", false, "whether the input topics should be parsed as topic IDs")
	cmd.Flags().BoolVarP(&pinternal, "internal", "i", false, "print internal topics if all topics are printed")
	cmd.Flags().BoolVarP(&detailed, "detailed", "d", false, "include detailed information about all topic partitions")
	cmd.Flags().BoolVarP(&pall, "all", "a", false, "shortcut for -cbti")
	return cmd
}

func printBrokers(controllerID int32, brokers []kmsg.MetadataResponseBroker) {
	sort.Slice(brokers, func(i, j int) bool {
		return brokers[i].NodeID < brokers[j].NodeID
	})

	tw := out.BeginTabWrite()
	defer tw.Flush()

	fmt.Fprintf(tw, "ID\tHOST\tPORT\tRACK\n")
	for _, broker := range brokers {
		var controllerStar string
		if broker.NodeID == controllerID {
			controllerStar = "*"
		}

		var rack string
		if broker.Rack != nil {
			rack = *broker.Rack
		}

		fmt.Fprintf(tw, "%d%s\t%s\t%d\t%s\n",
			broker.NodeID, controllerStar, broker.Host, broker.Port, rack)
	}
}

func printTopics(version int16, topics []kmsg.MetadataResponseTopic, pinternal, detailed bool) {
	sort.Slice(topics, func(i, j int) bool {
		l := topics[i].Topic
		r := topics[j].Topic
		switch {
		case l != nil && r != nil:
			return *l < *r
		case l != nil && r == nil:
			return true
		case r != nil:
			return false
		default:
			return string(topics[i].TopicID[:]) < string(topics[j].TopicID[:])
		}
	})

	hasID := version >= 10

	if !detailed {
		tw := out.BeginTabWrite()
		defer tw.Flush()

		if hasID {
			fmt.Fprintf(tw, "NAME\tID\tPARTITIONS\tREPLICAS\n")
		} else {
			fmt.Fprintf(tw, "NAME\tPARTITIONS\tREPLICAS\n")
		}
		for _, topic := range topics {
			if !pinternal && topic.IsInternal {
				continue
			}
			parts := len(topic.Partitions)
			replicas := 0
			if parts > 0 {
				replicas = len(topic.Partitions[0].Replicas)
			}
			if hasID {
				fmt.Fprintf(tw, "%s\t%x\t%d\t%d\n", topic.Topic, topic.TopicID, parts, replicas)
			} else {
				fmt.Fprintf(tw, "%s\t%d\t%d\n", topic.Topic, parts, replicas)
			}
		}
		tw.Flush()
		return
	}

	buf := new(bytes.Buffer)
	buf.Grow(10 << 10)
	defer func() { os.Stdout.Write(buf.Bytes()) }()

	for _, topic := range topics {
		fmt.Fprintf(buf, "%s", topic.Topic)
		if hasID {
			fmt.Fprintf(buf, " [%x]", topic.TopicID)
		}
		if topic.IsInternal {
			fmt.Fprint(buf, " (internal)")
		}

		parts := topic.Partitions
		fmt.Fprintf(buf, ", %d partition", len(parts))
		if len(parts) > 1 {
			buf.WriteByte('s')
		}
		buf.WriteString("\n")
		if topic.IsInternal && !pinternal {
			continue
		}

		sort.Slice(parts, func(i, j int) bool {
			return parts[i].Partition < parts[j].Partition
		})
		for _, part := range topic.Partitions {
			fmt.Fprintf(buf, "  %4d  leader %d replicas %v isr %v",
				part.Partition,
				part.Leader,
				part.Replicas,
				part.ISR,
			)
			if len(part.OfflineReplicas) > 0 {
				fmt.Fprintf(buf, ", offline replicas %v", part.OfflineReplicas)
			}
			if err := kerr.ErrorForCode(part.ErrorCode); err != nil {
				fmt.Fprintf(buf, " (%s)", err)
			}
			fmt.Fprintln(buf)
		}
	}
}
