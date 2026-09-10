package parse

import (
	"testing"

	"github.com/datarhei/core/v16/encoding/json"

	"github.com/stretchr/testify/require"
)

func TestCalculateMappingInputOutputSimple(t *testing.T) {
	// ffmpeg -i example_audio.mp4 -f null -
	inputline := []byte(`[{"url":"example_audio.mp4","format":"mov,mp4,m4a,3gp,3g2,mj2","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":1822,"duration_sec":10.005000,"language":"und","profile":578,"level":30,"disposition":["default"],"fps":25.000000,"pix_fmt":"yuv420p","width":640,"height":360},{"url":"example_audio.mp4","format":"mov,mp4,m4a,3gp,3g2,mj2","index":0,"stream":1,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":2,"duration_sec":10.005000,"language":"und","profile":1,"level":-99,"disposition":["default"],"sample_fmt":"fltp","sampling_hz":44100,"layout":"stereo","channels":2}]`)
	outputline := []byte(`[{"url":"pipe:","format":"null","index":0,"stream":0,"type":"video","codec":"wrapped_avframe","coder":"wrapped_avframe","bitrate_kbps":200,"duration_sec":0.000000,"language":"und","profile":-99,"level":-99,"disposition":["default"],"fps":25.000000,"pix_fmt":"yuv420p","width":640,"height":360},{"url":"pipe:","format":"null","index":0,"stream":1,"type":"audio","codec":"pcm_s16le","coder":"pcm_s16le","bitrate_kbps":1411,"duration_sec":0.000000,"language":"und","profile":-99,"level":-99,"disposition":["default"],"sample_fmt":"s16","sampling_hz":44100,"layout":"stereo","channels":2}]`)
	mappingline := []byte(`{"graphs":[{"index":0,"graph":[{"src_id":"71ac94f00","src_name":"Parsed_null_0","src_filter":"null","dst_id":"71ac95140","dst_name":"out_#0:0","dst_filter":"buffersink","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"71ac94fc0","src_name":"graph -1 input from stream 0:0","src_filter":"buffer","dst_id":"71ac94f00","dst_name":"Parsed_null_0","dst_filter":"null","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360}]},{"index":1,"graph":[{"src_id":"71ac94a80","src_name":"Parsed_anull_0","src_filter":"anull","dst_id":"71ac95200","dst_name":"auto_aresample_0","dst_filter":"aresample","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"stereo"},{"src_id":"71ac94b40","src_name":"graph_-1_in_0:1","src_filter":"abuffer","dst_id":"71ac94a80","dst_name":"Parsed_anull_0","dst_filter":"anull","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"stereo"},{"src_id":"71ac94e40","src_name":"format_out_#0:1","src_filter":"aformat","dst_id":"71ac94d80","dst_name":"out_#0:1","dst_filter":"abuffersink","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"s16","sampling_hz":44100,"layout":"stereo"},{"src_id":"71ac95200","src_name":"auto_aresample_0","src_filter":"aresample","dst_id":"71ac94e40","dst_name":"format_out_#0:1","dst_filter":"aformat","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"s16","sampling_hz":44100,"layout":"stereo"}]}],"mapping":[{"input":{"index":0,"stream":0},"graph":{"index":0,"id":"71ac94fc0","name":"graph -1 input from stream 0:0"},"output":null},{"input":{"index":0,"stream":1},"graph":{"index":1,"id":"71ac94b40","name":"graph_-1_in_0:1"},"output":null},{"input":null,"graph":{"index":0,"id":"71ac95140","name":"out_#0:0"},"output":{"index":0,"stream":0}},{"input":null,"graph":{"index":1,"id":"71ac94d80","name":"out_#0:1"},"output":{"index":0,"stream":1}}]}`)

	mapping := ffmpegStreamMapping{}

	err := json.Unmarshal(mappingline, &mapping)
	require.Nil(t, err)

	input := []ffmpegProcessIO{}

	err = json.Unmarshal(inputline, &input)
	require.Nil(t, err)

	output := []ffmpegProcessIO{}

	err = json.Unmarshal(outputline, &output)
	require.Nil(t, err)

	process := ffmpegProcess{
		input:   input,
		output:  output,
		mapping: mapping,
	}

	process.calculateMapping()

	require.Equal(t, map[int][]int{
		0: {0},
		1: {1},
	}, process.input2output)

	require.Equal(t, map[int][]int{
		0: {0},
		1: {1},
	}, process.output2input)
}

func TestCalculateMappingInputOutputComplex(t *testing.T) {
	// ffmpeg -i example_audio.mp4 -i example.jpg -filter_complex "[0:v]split=2[main][in1];[main]split=2[in2][in3];[in1]boxblur[blur],[blur][1:v]overlay[out1];[in2]negate[out2];[in3]yadif[out3];[out2]split=2[out20][out21]" -map "[out1]" -map "[out3]" -map "[out20]" -map "[out21]" -map 0:a -af "dcshift" -f matroska -y /dev/null
	inputline := []byte(`[{"url":"example_audio.mp4","format":"mov,mp4,m4a,3gp,3g2,mj2","index":0,"stream":0,"type":"video","codec":"h264","coder":"h264","bitrate_kbps":1822,"duration_sec":10.005000,"language":"und","profile":578,"level":30,"disposition":["default"],"fps":25.000000,"pix_fmt":"yuv420p","width":640,"height":360},{"url":"example_audio.mp4","format":"mov,mp4,m4a,3gp,3g2,mj2","index":0,"stream":1,"type":"audio","codec":"aac","coder":"aac","bitrate_kbps":2,"duration_sec":10.005000,"language":"und","profile":1,"level":-99,"disposition":["default"],"sample_fmt":"fltp","sampling_hz":44100,"layout":"stereo","channels":2},{"url":"example.jpg","format":"image2","index":1,"stream":0,"type":"video","codec":"mjpeg","coder":"mjpeg","bitrate_kbps":0,"duration_sec":0.045000,"language":"und","profile":192,"level":-99,"disposition":[],"fps":25.000000,"pix_fmt":"yuvj420p","width":640,"height":360}]`)
	outputline := []byte(`[{"url":"/dev/null","format":"matroska","index":0,"stream":0,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","profile":-99,"level":-99,"disposition":["default"],"fps":25.000000,"pix_fmt":"yuv420p","width":640,"height":360},{"url":"/dev/null","format":"matroska","index":0,"stream":1,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","profile":-99,"level":-99,"disposition":[],"fps":25.000000,"pix_fmt":"yuv420p","width":640,"height":360},{"url":"/dev/null","format":"matroska","index":0,"stream":2,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","profile":-99,"level":-99,"disposition":[],"fps":25.000000,"pix_fmt":"yuv420p","width":640,"height":360},{"url":"/dev/null","format":"matroska","index":0,"stream":3,"type":"video","codec":"h264","coder":"libx264","bitrate_kbps":0,"duration_sec":0.000000,"language":"und","profile":-99,"level":-99,"disposition":[],"fps":25.000000,"pix_fmt":"yuv420p","width":640,"height":360},{"url":"/dev/null","format":"matroska","index":0,"stream":4,"type":"audio","codec":"ac3","coder":"ac3","bitrate_kbps":192,"duration_sec":0.000000,"language":"und","profile":-99,"level":-99,"disposition":["default"],"sample_fmt":"fltp","sampling_hz":44100,"layout":"stereo","channels":2}]`)
	mappingline := []byte(`{"graphs":[{"index":0,"graph":[{"src_id":"c4b430fc0","src_name":"Parsed_split_0","src_filter":"split","dst_id":"c4b431740","dst_name":"Parsed_split_1","dst_filter":"split","inpad":"output0","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b430fc0","src_name":"Parsed_split_0","src_filter":"split","dst_id":"c4b431800","dst_name":"Parsed_boxblur_2","dst_filter":"boxblur","inpad":"output1","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431740","src_name":"Parsed_split_1","src_filter":"split","dst_id":"c4b431980","dst_name":"Parsed_negate_4","dst_filter":"negate","inpad":"output0","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431740","src_name":"Parsed_split_1","src_filter":"split","dst_id":"c4b431a40","dst_name":"Parsed_yadif_5","dst_filter":"yadif","inpad":"output1","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431800","src_name":"Parsed_boxblur_2","src_filter":"boxblur","dst_id":"c4b4318c0","dst_name":"Parsed_overlay_3","dst_filter":"overlay","inpad":"default","outpad":"main","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b4318c0","src_name":"Parsed_overlay_3","src_filter":"overlay","dst_id":"c4b432040","dst_name":"format","dst_filter":"format","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431980","src_name":"Parsed_negate_4","src_filter":"negate","dst_id":"c4b431bc0","dst_name":"Parsed_split_6","dst_filter":"split","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431a40","src_name":"Parsed_yadif_5","src_filter":"yadif","dst_id":"c4b4321c0","dst_name":"format","dst_filter":"format","inpad":"default","outpad":"default","timebase":"1/25600","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431bc0","src_name":"Parsed_split_6","src_filter":"split","dst_id":"c4b432340","dst_name":"format","dst_filter":"format","inpad":"output0","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431bc0","src_name":"Parsed_split_6","src_filter":"split","dst_id":"c4b4324c0","dst_name":"format","dst_filter":"format","inpad":"output1","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431c80","src_name":"graph 0 input from stream 0:0","src_filter":"buffer","dst_id":"c4b430fc0","dst_name":"Parsed_split_0","dst_filter":"split","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b431e00","src_name":"graph 0 input from stream 1:0","src_filter":"buffer","dst_id":"c4b432580","dst_name":"auto_scale_0","dst_filter":"scale","inpad":"default","outpad":"default","timebase":"1/25","type":"video","format":"yuvj420p","width":640,"height":360},{"src_id":"c4b432040","src_name":"format","src_filter":"format","dst_id":"c4b431f80","dst_name":"out_#0:0","dst_filter":"buffersink","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b4321c0","src_name":"format","src_filter":"format","dst_id":"c4b432100","dst_name":"out_#0:1","dst_filter":"buffersink","inpad":"default","outpad":"default","timebase":"1/25600","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b432340","src_name":"format","src_filter":"format","dst_id":"c4b432280","dst_name":"out_#0:2","dst_filter":"buffersink","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b4324c0","src_name":"format","src_filter":"format","dst_id":"c4b432400","dst_name":"out_#0:3","dst_filter":"buffersink","inpad":"default","outpad":"default","timebase":"1/12800","type":"video","format":"yuv420p","width":640,"height":360},{"src_id":"c4b432580","src_name":"auto_scale_0","src_filter":"scale","dst_id":"c4b4318c0","dst_name":"Parsed_overlay_3","dst_filter":"overlay","inpad":"default","outpad":"overlay","timebase":"1/25","type":"video","format":"yuva420p","width":640,"height":360}]},{"index":1,"graph":[{"src_id":"c4b431200","src_name":"Parsed_dcshift_0","src_filter":"dcshift","dst_id":"c4b431680","dst_name":"auto_aresample_1","dst_filter":"aresample","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"s32p","sampling_hz":44100,"layout":"stereo"},{"src_id":"c4b4312c0","src_name":"graph_-1_in_0:1","src_filter":"abuffer","dst_id":"c4b4315c0","dst_name":"auto_aresample_0","dst_filter":"aresample","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"stereo"},{"src_id":"c4b431500","src_name":"format_out_#0:4","src_filter":"aformat","dst_id":"c4b431440","dst_name":"out_#0:4","dst_filter":"abuffersink","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"stereo"},{"src_id":"c4b4315c0","src_name":"auto_aresample_0","src_filter":"aresample","dst_id":"c4b431200","dst_name":"Parsed_dcshift_0","dst_filter":"dcshift","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"s32p","sampling_hz":44100,"layout":"stereo"},{"src_id":"c4b431680","src_name":"auto_aresample_1","src_filter":"aresample","dst_id":"c4b431500","dst_name":"format_out_#0:4","dst_filter":"aformat","inpad":"default","outpad":"default","timebase":"1/44100","type":"audio","format":"fltp","sampling_hz":44100,"layout":"stereo"}]}],"mapping":[{"input":{"index":0,"stream":0},"graph":{"index":0,"id":"c4b431c80","name":"graph 0 input from stream 0:0"},"output":null},{"input":{"index":0,"stream":1},"graph":{"index":1,"id":"c4b4312c0","name":"graph_-1_in_0:1"},"output":null},{"input":{"index":1,"stream":0},"graph":{"index":0,"id":"c4b431e00","name":"graph 0 input from stream 1:0"},"output":null},{"input":null,"graph":{"index":0,"id":"c4b431f80","name":"out_#0:0"},"output":{"index":0,"stream":0}},{"input":null,"graph":{"index":0,"id":"c4b432100","name":"out_#0:1"},"output":{"index":0,"stream":1}},{"input":null,"graph":{"index":0,"id":"c4b432280","name":"out_#0:2"},"output":{"index":0,"stream":2}},{"input":null,"graph":{"index":0,"id":"c4b432400","name":"out_#0:3"},"output":{"index":0,"stream":3}},{"input":null,"graph":{"index":1,"id":"c4b431440","name":"out_#0:4"},"output":{"index":0,"stream":4}}]}`)

	mapping := ffmpegStreamMapping{}

	err := json.Unmarshal(mappingline, &mapping)
	require.Nil(t, err)

	input := []ffmpegProcessIO{}

	err = json.Unmarshal(inputline, &input)
	require.Nil(t, err)

	output := []ffmpegProcessIO{}

	err = json.Unmarshal(outputline, &output)
	require.Nil(t, err)

	process := ffmpegProcess{
		input:   input,
		output:  output,
		mapping: mapping,
	}

	process.calculateMapping()

	require.Equal(t, map[int][]int{
		0: {0, 1, 2, 3},
		1: {4},
		2: {0},
	}, process.input2output)

	require.Equal(t, map[int][]int{
		0: {0, 2},
		1: {0},
		2: {0},
		3: {0},
		4: {1},
	}, process.output2input)
}
