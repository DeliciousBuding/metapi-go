package messages

import "strings"

// Native images have no detail selector. Do not invent one or fetch remote
// pixels: a user image becomes one Chat image_url part at the same position.
func requestImage(block rawObject, path string) (wireObject, error) {
	if err := allowOnly(block, path, "type", "source", "cache_control"); err != nil {
		return nil, err
	}
	if !absent(block["cache_control"]) {
		cache, err := object(block["cache_control"], path+".cache_control")
		if err != nil {
			return nil, err
		}
		if len(cache) != 0 {
			return nil, invalid(path+".cache_control", "cannot preserve image cache controls through Chat")
		}
	}
	source, err := object(block["source"], path+".source")
	if err != nil {
		return nil, err
	}
	typ, err := text(source["type"], path+".source.type")
	if err != nil {
		return nil, err
	}
	var location string
	switch typ {
	case "base64":
		if err := allowOnly(source, path+".source", "type", "media_type", "data"); err != nil {
			return nil, err
		}
		media, err := text(source["media_type"], path+".source.media_type")
		if err != nil {
			return nil, err
		}
		data, err := identity(source["data"], path+".source.data")
		if err != nil {
			return nil, err
		}
		location = "data:" + media + ";base64," + data
	case "url":
		if err := allowOnly(source, path+".source", "type", "url"); err != nil {
			return nil, err
		}
		location, err = identity(source["url"], path+".source.url")
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(location, "data:") {
			return nil, invalid(path+".source.url", "must be an HTTP image URL; use base64 source for inline data")
		}
	default:
		return nil, unsupported(path + ".source.type")
	}
	if _, err := imageSourceForURL(location, path+".source"); err != nil {
		return nil, err
	}
	return wireObject{"type": "image_url", "image_url": wireObject{"url": location}}, nil
}
