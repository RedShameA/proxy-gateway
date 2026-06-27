package dictionaries

func CountryNameZH(code string) string {
	names := map[string]string{
		"US": "美国", "CN": "中国", "JP": "日本", "KR": "韩国", "SG": "新加坡",
		"HK": "香港", "TW": "台湾", "GB": "英国", "DE": "德国", "FR": "法国",
		"NL": "荷兰", "RU": "俄罗斯", "CA": "加拿大", "AU": "澳大利亚", "IN": "印度",
		"BR": "巴西", "TH": "泰国", "VN": "越南", "MY": "马来西亚", "ID": "印度尼西亚",
		"PH": "菲律宾", "AE": "阿联酋", "SA": "沙特", "TR": "土耳其", "IL": "以色列",
		"UA": "乌克兰", "PL": "波兰", "IT": "意大利", "ES": "西班牙", "PT": "葡萄牙",
		"SE": "瑞典", "NO": "挪威", "FI": "芬兰", "DK": "丹麦", "CH": "瑞士",
		"AT": "奥地利", "BE": "比利时", "IE": "爱尔兰", "NZ": "新西兰", "MX": "墨西哥",
		"AR": "阿根廷", "CL": "智利", "CO": "哥伦比亚", "ZA": "南非", "EG": "埃及",
	}
	if name, ok := names[code]; ok {
		return name
	}
	return code
}
