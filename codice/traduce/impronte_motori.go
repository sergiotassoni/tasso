package traduce

// Impronte SHA-256 dei file estratti dagli zip ufficiali (già verificati): prima di avviare un
// motore controllo che nella sua cartella ci siano solo questi file, uguali a quelli originali.
// llama.cpp b12189 Vulkan (zip 9d564055…) e whisper.cpp b5454 x64 (zip 6ba69e34…), calcolate il 22/11/2026.

var improntaLlama = map[string]string{
	"ggml-base.dll":                "ce41235a69d0b1c8488102f17254291d203932c7ea4a9e3ee00deb96c3ff56fa",
	"ggml-cpu-alderlake.dll":       "1da95d9cf5b3d17a7c8567cf33a5de02b75bbe13e17dd5ee496d1490a7d0f083",
	"ggml-cpu-cannonlake.dll":      "b619d463a7af9e609ccfbad12943cbef2935d71f305b3c34360ca8701eb9b9ac",
	"ggml-cpu-cascadelake.dll":     "771f895d380b130b715bf0866fdaf7636ccac01e0fab04ff89256d33b100c277",
	"ggml-cpu-cooperlake.dll":      "b225830a78d2d356f7d51e48f1e0abfbb286465a7a60283776bbd72bbf93af1c",
	"ggml-cpu-haswell.dll":         "5059cc8759c1b4aa143df7f4f57e56c05af0a1db65e28fbfd536c6345f5edc8e",
	"ggml-cpu-icelake.dll":         "3bd5b487f5f76cfa61b8935f4a80775b41f3e7869fd70a54749520be092c18fd",
	"ggml-cpu-ivybridge.dll":       "9996ec0300e9749a953e71145a9346f7bd38b4789ee7e82f635c3b7392c9328f",
	"ggml-cpu-piledriver.dll":      "e60311d872497db93c3625aef17f14b85bce3ba5a8effb5ddfe9121f6dcf7fc4",
	"ggml-cpu-sandybridge.dll":     "75decb89a13b0bd933416b7c1f44998462c721b5291017ea7419722e59005997",
	"ggml-cpu-sapphirerapids.dll":  "2ebf2f70688cd283774cb7da43afb24e170ab5f6640cfdfaabe9bc19b41eb938",
	"ggml-cpu-skylakex.dll":        "869847f969b8a45ca80ea655557124f3b1801ce8f625fb6a606e4f3346438768",
	"ggml-cpu-sse42.dll":           "3e8f6a862ceda627a3b246098a5a39c99e393a04b6c4a7289bf69428b764733d",
	"ggml-cpu-x64.dll":             "4973084d5f0a9669cad5768c82bf7ce03c6850a82b364747361060e765851524",
	"ggml-cpu-zen4.dll":            "7f7519552c326b35c67a9666281c4bb14574614b06af3889643f329f0107e8c9",
	"ggml-rpc.dll":                 "a44833c5eeddf557d1b654bf12a0eb83cfe0b61ee9ea9c15b0c0e899b998863d",
	"ggml-vulkan.dll":              "b3bdbdddb4de29ed93b4593f3527481d07248e9524f5f6218df0e675afde437c",
	"ggml.dll":                     "a117aa4c823a52dfdfc4023a453af6102dae9a09198404c6299e6b872908e458",
	"libomp.dll":                   "a12116ba72d1d6820407cf30be23da04ce79d6bb8a71a5ee71759c5a1faa6f1c",
	"llama-batched-bench-impl.dll": "8ce258204752530539b981dec364853ad86ba6adc14a6b8e3dfd43a699ec631b",
	"llama-bench-impl.dll":         "7f2c124113a606075afefa0d037362270f009a05771407a3c2a25d149f9b6ccc",
	"llama-cli-impl.dll":           "bfaae0f17a02e0fc59ecff76d8391b944318b33328d69bdb08dc04e3d677862b",
	"llama-common.dll":             "f7fdaeac088b9286772375fcd1b14b55819f02f7cb73411530d62a9a5214a76a",
	"llama-completion-impl.dll":    "b406a7eb470965d213fdd07af392154021d07a1447994ce4965d47b454784c6a",
	"llama-fit-params-impl.dll":    "8545e649e5f17d704b01e57629e28305d30ab37945b114f213867742dfb010b2",
	"llama-perplexity-impl.dll":    "c5b232170a44ab2ff9c91fd6dcdd5095d2da007c631a5320a5463441ca4cf2a4",
	"llama-quantize-impl.dll":      "a70e3749a5247fe001792f571d29dc609886d44a35175a7a0b8d8a6b5edd801a",
	"llama-server-impl.dll":        "99f652aef1f87a00502868faec9279f9285788ffb77e5566485a113a5882ce45",
	"llama-server.exe":             "ea684e660301cdcf1eca251e49c03782c126c7f832f934624a693a187520498c",
	"llama.dll":                    "bf2e26b996a08b4d8ba5a91b1d709928b900049983f724a243a92d365e4f6a39",
	"mtmd.dll":                     "6dc418a2d453e1811d232cae134945bf09ef5fbfde7b6f85fefd18f4040c7a5a",
}

var improntaWhisper = map[string]string{
	"whisper-server.exe":       "a97456e747f28e7bb09c1dac35f40d94404c5ecffa74aaaaa7a3753ea6543121",
	"whisper.dll":              "034396d75bb5720c1851674ff64a96192b58f6927b68d334ebd254ce6cad9674",
	"ggml-base.dll":            "bede0d6b2d387236ecd2b5dd676d51c8984b5b6e0ae461a12174e9c01345c5f0",
	"ggml-cpu-alderlake.dll":   "d8f5c56ea490394eeae8b271650f2647aecd039a542d6df9e0140726e62a4603",
	"ggml-cpu-cannonlake.dll":  "74127b40abd4f595d9db6af0fa313a02e18ff42fc228bfa05281e7c788dd4507",
	"ggml-cpu-cascadelake.dll": "4b11c427b844eeac87529af9a8d94b35aad2024666beb2a35e59fe3077e4b81e",
	"ggml-cpu-haswell.dll":     "1306b1014c8118d27234d8ddc0e66ced1f8c8476bf85071595fd182ea9b7263b",
	"ggml-cpu-icelake.dll":     "2b4dc4c66e7d8fe2807283816e9cf3b7f7d0556f59df34069871e37eab8aed40",
	"ggml-cpu-sandybridge.dll": "04edbdebc01a1d421062a960881a4f34d520d99693999518243f332406444c6e",
	"ggml-cpu-skylakex.dll":    "1774714cf93621406ff35344b54715619a83242ae3e331e44d98a3b35ce801be",
	"ggml-cpu-sse42.dll":       "f060318772c4c2dbac312bbfff6ea700b85dbc502bb38585a7186d06d549301a",
	"ggml-cpu-x64.dll":         "4b4b05caba40d4309b5d710ef74c59ba190516b8489f27bdb1ba11ae33c6e9d3",
	"ggml.dll":                 "31fe18616d872af9b6134b47a6dff8072ae4ecbe5df0273f4f8dc36766c9db8e",
}
