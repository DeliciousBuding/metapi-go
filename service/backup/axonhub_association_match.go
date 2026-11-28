package backup

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type axonHubConnection struct {
	channelSourceID int
	modelName       string
	requestModel    string
}

// matchAxonHubAssociation is a faithful port of AxonHub's six association
// matchers. The second result reports a structural error (an unknown type or an
// unusable regex) rather than an empty match.
func matchAxonHubAssociation(assoc AxonHubSourceAssociation, channels []*axonHubPlanChannel, bySourceID map[int]*axonHubPlanChannel) ([]axonHubConnection, bool) {
	switch assoc.Type {
	case "channel_model":
		if assoc.ChannelModel == nil {
			return nil, false
		}
		channel := bySourceID[assoc.ChannelModel.ChannelID]
		if channel == nil {
			return nil, true
		}
		if _, ok := channel.Entries[assoc.ChannelModel.ModelID]; !ok {
			return nil, true
		}
		return []axonHubConnection{{assoc.ChannelModel.ChannelID, channel.Entries[assoc.ChannelModel.ModelID], assoc.ChannelModel.ModelID}}, true
	case "channel_regex":
		if assoc.ChannelRegex == nil {
			return nil, false
		}
		channel := bySourceID[assoc.ChannelRegex.ChannelID]
		if channel == nil {
			return nil, true
		}
		matcher, err := compileAxonHubPattern(assoc.ChannelRegex.Pattern)
		if err != nil {
			return nil, false
		}
		return matchEntries(channel.SourceID, channel.Entries, matcher), true
	case "regex":
		if assoc.Regex == nil {
			return nil, false
		}
		matcher, err := compileAxonHubPattern(assoc.Regex.Pattern)
		if err != nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if axonHubExcluded(channel, assoc.Regex.Exclude) {
				continue
			}
			out = append(out, matchEntries(channel.SourceID, channel.Entries, matcher)...)
		}
		return out, true
	case "model":
		if assoc.ModelID == nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if axonHubExcluded(channel, assoc.ModelID.Exclude) {
				continue
			}
			if actual, ok := channel.Entries[assoc.ModelID.ModelID]; ok {
				out = append(out, axonHubConnection{channel.SourceID, actual, assoc.ModelID.ModelID})
			}
		}
		return out, true
	case "channel_tags_model":
		if assoc.ChannelTagsModel == nil || len(assoc.ChannelTagsModel.ChannelTags) == 0 {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if !channelHasAnyTag(channel, assoc.ChannelTagsModel.ChannelTags) {
				continue
			}
			if actual, ok := channel.Entries[assoc.ChannelTagsModel.ModelID]; ok {
				out = append(out, axonHubConnection{channel.SourceID, actual, assoc.ChannelTagsModel.ModelID})
			}
		}
		return out, true
	case "channel_tags_regex":
		if assoc.ChannelTagsRegex == nil || len(assoc.ChannelTagsRegex.ChannelTags) == 0 {
			return nil, false
		}
		matcher, err := compileAxonHubPattern(assoc.ChannelTagsRegex.Pattern)
		if err != nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if !channelHasAnyTag(channel, assoc.ChannelTagsRegex.ChannelTags) {
				continue
			}
			out = append(out, matchEntries(channel.SourceID, channel.Entries, matcher)...)
		}
		return out, true
	default:
		return nil, false
	}
}

func matchEntries(channelSourceID int, entries map[string]string, matcher func(string) bool) []axonHubConnection {
	names := make([]string, 0, len(entries))
	for requestModel := range entries {
		if matcher(requestModel) {
			names = append(names, requestModel)
		}
	}
	sort.Strings(names)
	out := make([]axonHubConnection, 0, len(names))
	for _, name := range names {
		out = append(out, axonHubConnection{channelSourceID, entries[name], name})
	}
	return out
}

// compileAxonHubPattern uses Go's regexp engine. AxonHub's xregexp wrapper is
// RE2-compatible, so a pattern that fails to compile here is refused rather
// than approximated.
func compileAxonHubPattern(pattern string) (func(string) bool, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	expr, err := regexp.Compile(trimmed)
	if err != nil {
		return nil, err
	}
	return expr.MatchString, nil
}

func channelHasAnyTag(channel *axonHubPlanChannel, tags []string) bool {
	for _, tag := range tags {
		for _, candidate := range channel.Tags {
			if candidate == tag {
				return true
			}
		}
	}
	return false
}

func axonHubExcluded(channel *axonHubPlanChannel, excludes []AxonHubSourceExclude) bool {
	for _, exclude := range excludes {
		if exclude.ChannelNamePattern != "" {
			if matcher, err := compileAxonHubPattern(exclude.ChannelNamePattern); err == nil && matcher(channel.Name) {
				return true
			}
		}
		for _, id := range exclude.ChannelIDs {
			if id == channel.SourceID {
				return true
			}
		}
		for _, tag := range exclude.ChannelTags {
			for _, candidate := range channel.Tags {
				if candidate == tag {
					return true
				}
			}
		}
	}
	return false
}
