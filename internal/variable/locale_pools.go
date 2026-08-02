package variable

import (
	"fmt"
	"math/rand/v2"
)

// enGBLocale returns the en-GB pool. Phone shape per SPEC:974 "+44 20 7946 0958".
func enGBLocale() *localeData {
	return &localeData{
		code: "en-GB",
		firstNames: []string{
			"Oliver", "Jack", "Harry", "George", "Noah", "Charlie", "Jacob",
			"Alfie", "Freddie", "Oscar", "Olivia", "Amelia", "Isla", "Ava",
			"Emily", "Isabella", "Poppy", "Ella", "Chloe", "Lily", "William",
			"James", "Thomas", "Henry", "Edward", "Arthur", "Archie", "Leo",
			"Theo", "Ethan", "Grace", "Sophie", "Mia", "Jessica", "Ruby",
		},
		lastNames: []string{
			"Smith", "Jones", "Williams", "Taylor", "Brown", "Davies", "Evans",
			"Wilson", "Thomas", "Roberts", "Walker", "Wright", "Robinson", "Thompson",
			"White", "Hughes", "Edwards", "Green", "Hall", "Wood", "Harris", "Lewis",
			"Martin", "Jackson", "Clarke", "Clark", "Turner", "Hill", "Scott", "Moore",
		},
		cities: []string{
			"London", "Birmingham", "Manchester", "Leeds", "Glasgow", "Liverpool",
			"Bristol", "Sheffield", "Edinburgh", "Leicester", "Coventry", "Bradford",
			"Cardiff", "Belfast", "Nottingham", "Newcastle", "Southampton", "Portsmouth",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +44 20 7946 0958 → +44 <2-digit area> <4-digit> <4-digit>
			return fmt.Sprintf("+44 %02d %04d %04d",
				intn(rng, 90)+10, intn(rng, 10000), intn(rng, 10000))
		},
	}
}

// frFRLocale returns the fr-FR pool. Phone shape per SPEC:976 "+33 1 23 45 67 89".
func frFRLocale() *localeData {
	return &localeData{
		code: "fr-FR",
		firstNames: []string{
			"Jean", "Marie", "Pierre", "Sophie", "Luc", "Camille", "Louis",
			"Emma", "Hugo", "Chloé", "Antoine", "Léa", "Nicolas", "Manon",
			"Julien", "Sarah", "Thomas", "Inès", "Maxime", "Jade", "Alexandre",
			"Louise", "Romain", "Alice", "Mathieu", "Lucie", "Clément",
			"Juliette", "Baptiste", "Zoé",
		},
		lastNames: []string{
			"Martin", "Bernard", "Dubois", "Thomas", "Robert", "Richard",
			"Petit", "Durand", "Leroy", "Moreau", "Simon", "Laurent", "Lefebvre",
			"Michel", "Garcia", "David", "Bertrand", "Roux", "Vincent", "Fournier",
			"Morel", "Girard", "André", "Lefèvre", "Mercier", "Dupont", "Lambert",
			"Bonnet", "François", "Martinez",
		},
		cities: []string{
			"Paris", "Marseille", "Lyon", "Toulouse", "Nice", "Nantes",
			"Strasbourg", "Montpellier", "Bordeaux", "Lille", "Rennes",
			"Reims", "Toulon", "Grenoble", "Dijon",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +33 1 23 45 67 89 → +33 <1-digit area> then four 2-digit groups
			return fmt.Sprintf("+33 %d %02d %02d %02d %02d",
				intn(rng, 9)+1, intn(rng, 100), intn(rng, 100),
				intn(rng, 100), intn(rng, 100))
		},
	}
}

// esESLocale returns the es-ES pool. Phone shape per SPEC:977 "+34 91 234 56 78".
func esESLocale() *localeData {
	return &localeData{
		code: "es-ES",
		firstNames: []string{
			"Antonio", "Manuel", "José", "Francisco", "David", "Juan", "Javier",
			"Daniel", "Carlos", "Alejandro", "María", "Carmen", "Ana", "Isabel",
			"Lucía", "Sara", "Laura", "Paula", "Marta", "Elena", "Miguel",
			"Pedro", "Luis", "Álvaro", "Sergio", "Raúl", "Pablo", "Jorge",
			"Cristina", "Patricia",
		},
		lastNames: []string{
			"García", "Martínez", "López", "Sánchez", "González", "Pérez",
			"Rodríguez", "Fernández", "Jiménez", "Díaz", "Ruiz", "Hernández",
			"Moreno", "Muñoz", "Álvarez", "Romero", "Alonso", "Gutiérrez",
			"Navarro", "Torres", "Domínguez", "Vázquez", "Ramos", "Gil", "Serrano",
			"Blanco", "Molina", "Morales", "Suárez", "Ortega",
		},
		cities: []string{
			"Madrid", "Barcelona", "Valencia", "Sevilla", "Zaragoza", "Málaga",
			"Murcia", "Palma", "Bilbao", "Alicante", "Córdoba", "Valladolid",
			"Vigo", "Gijón", "Pamplona",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +34 91 234 56 78 → +34 <2-digit area> <3-digit> <2-digit> <2-digit>
			return fmt.Sprintf("+34 %02d %03d %02d %02d",
				intn(rng, 90)+10, intn(rng, 1000), intn(rng, 100), intn(rng, 100))
		},
	}
}

// itITLocale returns the it-IT pool. Phone shape per SPEC:978 "+39 02 1234 5678".
func itITLocale() *localeData {
	return &localeData{
		code: "it-IT",
		firstNames: []string{
			"Marco", "Luca", "Andrea", "Francesco", "Alessandro", "Matteo", "Lorenzo",
			"Davide", "Riccardo", "Stefano", "Sofia", "Giulia", "Sara", "Martina",
			"Valentina", "Federica", "Chiara", "Alessia", "Silvia", "Elisa",
			"Giovanni", "Antonio", "Giuseppe", "Mario", "Roberto", "Simone",
			"Gabriele", "Nicola", "Emanuele", "Gianluca",
		},
		lastNames: []string{
			"Rossi", "Russo", "Ferrari", "Esposito", "Bianchi", "Romano", "Colombo",
			"Ricci", "Marino", "Greco", "Bruno", "Gallo", "Conti", "De Luca",
			"Costa", "Giordano", "Mancini", "Rizzo", "Lombardi", "Moretti",
			"Barbieri", "Fontana", "Santoro", "Marini", "Rinaldi", "Caruso",
			"Ferrara", "Galli", "Ferraro", "Leone",
		},
		cities: []string{
			"Roma", "Milano", "Napoli", "Torino", "Palermo", "Genova", "Bologna",
			"Firenze", "Bari", "Catania", "Venezia", "Verona", "Messina", "Padova",
			"Trieste",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +39 02 1234 5678 → +39 <2-digit area> <4-digit> <4-digit>
			return fmt.Sprintf("+39 %02d %04d %04d",
				intn(rng, 90)+10, intn(rng, 10000), intn(rng, 10000))
		},
	}
}

// ptBRLocale returns the pt-BR pool. Phone shape per SPEC:979 "+55 11 91234-5678".
func ptBRLocale() *localeData {
	return &localeData{
		code: "pt-BR",
		firstNames: []string{
			"João", "Pedro", "Lucas", "Gabriel", "Mateus", "Rafael", "Bruno",
			"Felipe", "Thiago", "Gustavo", "Maria", "Ana", "Juliana", "Fernanda",
			"Amanda", "Camila", "Larissa", "Vanessa", "Patricia", "Gabriela",
			"Carlos", "Paulo", "Ricardo", "Fernando", "Rodrigo", "Marcelo",
			"Eduardo", "Leonardo", "Diego", "Henrique",
		},
		lastNames: []string{
			"Silva", "Santos", "Oliveira", "Souza", "Rodrigues", "Ferreira",
			"Alves", "Pereira", "Lima", "Carvalho", "Melo", "Ribeiro", "Almeida",
			"Nascimento", "Costa", "Machado", "Nunes", "Gomes", "Martins", "Barbosa",
			"Rocha", "Moreira", "Lopes", "Mendes", "Araújo", "Cardoso", "Teixeira",
			"Moura", "Ramos", "Vieira",
		},
		cities: []string{
			"São Paulo", "Rio de Janeiro", "Brasília", "Salvador", "Fortaleza",
			"Belo Horizonte", "Manaus", "Curitiba", "Recife", "Porto Alegre",
			"Goiânia", "Belém", "Guarulhos", "Campinas", "São Luís",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +55 11 91234-5678 → +55 <2-digit DDD> <5-digit> - <4-digit>
			return fmt.Sprintf("+55 %02d %05d-%04d",
				intn(rng, 90)+10, intn(rng, 100000), intn(rng, 10000))
		},
	}
}

// nlNLLocale returns the nl-NL pool. Phone shape per SPEC:980 "+31 20 123 4567".
func nlNLLocale() *localeData {
	return &localeData{
		code: "nl-NL",
		firstNames: []string{
			"Jan", "Piet", "Klaas", "Hendrik", "Johan", "Willem", "Pieter",
			"Gerrit", "Arie", "Cornelis", "Anna", "Maria", "Johanna", "Elisabeth",
			"Margaretha", "Emma", "Lotte", "Fleur", "Inge", "Femke", "Thomas",
			"Daan", "Sander", "Tim", "Bram", "Niels", "Jasper", "Bas", "Joris", "Lars",
		},
		lastNames: []string{
			"de Jong", "Jansen", "de Vries", "van den Berg", "van Dijk", "Bakker",
			"Janssen", "Visser", "Smit", "Meijer", "de Boer", "Mulder", "de Groot",
			"Bos", "Vos", "Peters", "Hendriks", "van Leeuwen", "Dekker", "Brouwer",
			"de Wit", "Dijkstra", "Smits", "van der Berg", "van der Meer", "Kok",
			"Jacobs", "Willems", "de Graaf", "van Dam",
		},
		cities: []string{
			"Amsterdam", "Rotterdam", "Den Haag", "Utrecht", "Eindhoven",
			"Groningen", "Tilburg", "Almere", "Breda", "Nijmegen",
			"Haarlem", "Arnhem", "Enschede", "Apeldoorn", "Zaanstad",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +31 20 123 4567 → +31 <2-digit area> <3-digit> <4-digit>
			return fmt.Sprintf("+31 %02d %03d %04d",
				intn(rng, 90)+10, intn(rng, 1000), intn(rng, 10000))
		},
	}
}

// plPLLocale returns the pl-PL pool. Phone shape per SPEC:981 "+48 12 345 67 89".
func plPLLocale() *localeData {
	return &localeData{
		code: "pl-PL",
		firstNames: []string{
			"Andrzej", "Piotr", "Krzysztof", "Tomasz", "Jan", "Paweł", "Marek",
			"Marcin", "Michał", "Rafał", "Anna", "Maria", "Katarzyna", "Małgorzata",
			"Agnieszka", "Barbara", "Ewa", "Elżbieta", "Joanna", "Zofia",
			"Adam", "Łukasz", "Dariusz", "Mariusz", "Grzegorz", "Robert",
			"Jakub", "Mateusz", "Kamil", "Bartosz",
		},
		lastNames: []string{
			"Nowak", "Kowalski", "Wiśniewski", "Wójcik", "Kowalczyk", "Kamiński",
			"Lewandowski", "Zieliński", "Woźniak", "Szymański", "Dąbrowski",
			"Kozłowski", "Jankowski", "Mazur", "Wojciechowski", "Kwiatkowski",
			"Krawczyk", "Kaczmarek", "Piotrowski", "Grabowski", "Nowakowski",
			"Pawłowski", "Michalski", "Nowicki", "Adamczyk", "Dudek",
			"Zajączkowski", "Wieczorek", "Jabłoński", "Królik",
		},
		cities: []string{
			"Warszawa", "Kraków", "Łódź", "Wrocław", "Poznań", "Gdańsk",
			"Szczecin", "Bydgoszcz", "Lublin", "Katowice", "Białystok",
			"Gdynia", "Częstochowa", "Radom", "Sosnowiec",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +48 12 345 67 89 → +48 <2-digit area> <3-digit> <2-digit> <2-digit>
			return fmt.Sprintf("+48 %02d %03d %02d %02d",
				intn(rng, 90)+10, intn(rng, 1000), intn(rng, 100), intn(rng, 100))
		},
	}
}

// svSELocale returns the sv-SE pool. Phone shape per SPEC:982 "+46 8 123 45 67".
func svSELocale() *localeData {
	return &localeData{
		code: "sv-SE",
		firstNames: []string{
			"Erik", "Lars", "Karl", "Anders", "Johan", "Per", "Nils", "Lennart",
			"Gunnar", "Olof", "Marie", "Anna", "Karin", "Britta", "Elisabeth",
			"Eva", "Birgitta", "Margareta", "Kristina", "Maria", "Sven", "Magnus",
			"Mikael", "Stefan", "Peter", "Henrik", "Andreas", "Jonas", "Mattias", "Ola",
		},
		lastNames: []string{
			"Johansson", "Andersson", "Karlsson", "Nilsson", "Eriksson", "Larsson",
			"Olsson", "Persson", "Svensson", "Gustafsson", "Pettersson", "Jonsson",
			"Jansson", "Hansson", "Bengtsson", "Jönsson", "Lindberg", "Jakobsson",
			"Magnusson", "Olofsson", "Lindgren", "Axelsson", "Berg", "Lundberg",
			"Lindqvist", "Holm", "Lindström", "Engström", "Fredriksson", "Sandberg",
		},
		cities: []string{
			"Stockholm", "Göteborg", "Malmö", "Uppsala", "Västerås", "Örebro",
			"Linköping", "Helsingborg", "Jönköping", "Norrköping", "Lund",
			"Umeå", "Gävle", "Borås", "Eskilstuna",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +46 8 123 45 67 → +46 <1-digit area> <3-digit> <2-digit> <2-digit>
			return fmt.Sprintf("+46 %d %03d %02d %02d",
				intn(rng, 9)+1, intn(rng, 1000), intn(rng, 100), intn(rng, 100))
		},
	}
}

// trTRLocale returns the tr-TR pool. Phone shape per SPEC:983 "+90 212 345 67 89".
// Turkish-specific Unicode codepoints (ı U+0131, İ U+0130, ş U+015F, ç U+00E7,
// ö U+00F6, ü U+00FC, ğ U+011F) are stored verbatim; no casing transform is applied.
func trTRLocale() *localeData {
	return &localeData{
		code: "tr-TR",
		firstNames: []string{
			"Ahmet", "Mehmet", "Mustafa", "Ali", "Hüseyin", "Hasan", "İbrahim",
			"İsmail", "Ömer", "Yusuf", "Fatma", "Ayşe", "Emine", "Hatice",
			"Zeynep", "Elif", "Meryem", "Şule", "Esra", "Sibel", "Emre",
			"Burak", "Murat", "Serkan", "Kemal", "Oğuzhan", "Tolga", "Berk",
			"Çağrı", "Arda",
		},
		lastNames: []string{
			"Yılmaz", "Kaya", "Demir", "Şahin", "Çelik", "Yıldız", "Yıldırım",
			"Öztürk", "Aydın", "Özdemir", "Arslan", "Doğan", "Kılıç", "Aslan",
			"Çetin", "Kara", "Koç", "Kurt", "Özkan", "Şimşek", "Erdoğan",
			"Bulut", "Korkmaz", "Ateş", "Polat", "Çakır", "Güneş", "Aksoy",
			"Karahan", "Tekin",
		},
		cities: []string{
			"İstanbul", "Ankara", "İzmir", "Bursa", "Adana", "Gaziantep",
			"Konya", "Antalya", "Mersin", "Diyarbakır", "Kayseri", "Eskişehir",
			"Şanlıurfa", "Trabzon", "Samsun",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +90 212 345 67 89 → +90 <3-digit area> <3-digit> <2-digit> <2-digit>
			return fmt.Sprintf("+90 %03d %03d %02d %02d",
				intn(rng, 900)+100, intn(rng, 1000), intn(rng, 100), intn(rng, 100))
		},
	}
}

// jaJPLocale returns the ja-JP pool. Family-name-first (e.g. "山田 太郎").
// Phone shape per SPEC:980 table "+81 3-1234-5678".
func jaJPLocale() *localeData {
	return &localeData{
		code:            "ja-JP",
		familyNameFirst: true,
		firstNames: []string{
			"太郎", "花子", "一郎", "幸子", "次郎", "良子", "健一", "雅子",
			"義男", "啓子", "浩二", "典子", "修", "友子", "博", "千代",
			"光男", "恵子", "茂", "直子", "勉", "里美", "隆", "美奈子",
			"昭", "陽子", "清", "和子", "進", "由美子",
		},
		lastNames: []string{
			"山田", "佐藤", "鈴木", "田中", "渡辺", "伊藤", "中村", "小林",
			"加藤", "吉田", "山口", "松本", "井上", "木村", "林", "斎藤",
			"清水", "山本", "中島", "石川", "前田", "小川", "岡田", "後藤",
			"長谷川", "村田", "近藤", "石田", "藤田", "橋本",
		},
		cities: []string{
			"東京", "大阪", "名古屋", "横浜", "札幌", "福岡", "神戸", "京都",
			"仙台", "広島", "川崎", "埼玉", "千葉", "北九州", "浜松",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +81 3-1234-5678 → +81 <1-digit area> - <4-digit> - <4-digit>
			return fmt.Sprintf("+81 %d-%04d-%04d",
				intn(rng, 9)+1, intn(rng, 10000), intn(rng, 10000))
		},
	}
}

// zhCNLocale returns the zh-CN pool. Family-name-first (e.g. "张伟").
// Phone shape per SPEC:981 table "+86 10 1234 5678".
func zhCNLocale() *localeData {
	return &localeData{
		code:            "zh-CN",
		familyNameFirst: true,
		firstNames: []string{
			"伟", "芳", "娜", "秀英", "敏", "静", "丽", "强",
			"磊", "洋", "艳", "勇", "军", "杰", "娟", "涛",
			"明", "超", "秀兰", "霞", "平", "刚", "桂英", "华",
			"彬", "建华", "宇", "丹", "晓", "志强",
		},
		lastNames: []string{
			"张", "王", "李", "赵", "刘", "陈", "杨", "黄",
			"周", "吴", "徐", "孙", "胡", "朱", "高",
			"林", "何", "郭", "马", "罗", "梁", "宋", "郑", "谢",
			"韩", "唐", "冯", "于", "董", "曹",
		},
		cities: []string{
			"北京", "上海", "广州", "深圳", "成都", "武汉", "杭州", "西安",
			"南京", "天津", "重庆", "沈阳", "济南", "长沙", "哈尔滨",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +86 10 1234 5678 → +86 <2-digit area> <4-digit> <4-digit>
			return fmt.Sprintf("+86 %02d %04d %04d",
				intn(rng, 90)+10, intn(rng, 10000), intn(rng, 10000))
		},
	}
}

// koKRLocale returns the ko-KR pool. Family-name-first (e.g. "김민준").
// Phone shape per SPEC:982 table "+82 2-1234-5678".
func koKRLocale() *localeData {
	return &localeData{
		code:            "ko-KR",
		familyNameFirst: true,
		firstNames: []string{
			"민준", "서연", "지우", "서준", "하은", "도윤", "수아", "주원",
			"유나", "지훈", "민서", "시우", "지아", "준서", "예은", "채원",
			"지민", "예린", "태양", "소연", "현우", "유진", "정우", "수빈",
			"지현", "선우", "다은", "규민", "나연", "성민",
		},
		lastNames: []string{
			"김", "이", "박", "최", "정", "강", "조", "윤",
			"장", "임", "한", "오", "서", "신", "권",
			"황", "안", "송", "류", "전", "홍", "고", "문", "양",
			"손", "배", "백", "허", "유", "남",
		},
		cities: []string{
			"서울", "부산", "인천", "대구", "대전", "광주", "수원", "울산",
			"창원", "고양", "용인", "성남", "청주", "전주", "안산",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +82 2-1234-5678 → +82 <1-digit area> - <4-digit> - <4-digit>
			return fmt.Sprintf("+82 %d-%04d-%04d",
				intn(rng, 9)+1, intn(rng, 10000), intn(rng, 10000))
		},
	}
}

// ruRULocale returns the ru-RU pool. Given-name-first (e.g. "Иван Иванов").
// Phone shape per SPEC:985 table "+7 495 123-45-67".
func ruRULocale() *localeData {
	return &localeData{
		code: "ru-RU",
		// familyNameFirst stays false — given-name-first like Latin locales.
		firstNames: []string{
			"Иван", "Мария", "Алексей", "Елена", "Дмитрий", "Ольга", "Андрей",
			"Наталья", "Сергей", "Татьяна", "Михаил", "Ирина", "Александр",
			"Светлана", "Николай", "Анна", "Павел", "Галина", "Артём", "Людмила",
			"Виктор", "Нина", "Евгений", "Тамара", "Владимир", "Валентина",
			"Максим", "Зинаида", "Пётр", "Надежда",
		},
		lastNames: []string{
			"Иванов", "Смирнов", "Кузнецов", "Попов", "Васильев", "Петров",
			"Соколов", "Михайлов", "Новиков", "Фёдоров", "Морозов", "Волков",
			"Алексеев", "Лебедев", "Семёнов", "Егоров", "Павлов", "Козлов",
			"Степанов", "Николаев", "Орлов", "Андреев", "Макаров", "Никитин",
			"Захаров", "Зайцев", "Соловьёв", "Борисов", "Яковлев", "Григорьев",
		},
		cities: []string{
			"Москва", "Санкт-Петербург", "Новосибирск", "Екатеринбург", "Казань",
			"Нижний Новгород", "Челябинск", "Самара", "Уфа", "Ростов-на-Дону",
			"Красноярск", "Воронеж", "Пермь", "Волгоград", "Краснодар",
		},
		phoneFormat: func(rng *rand.Rand) string {
			// +7 495 123-45-67 → +7 <3-digit area> <3-digit>-<2-digit>-<2-digit>
			return fmt.Sprintf("+7 %03d %03d-%02d-%02d",
				intn(rng, 900)+100, intn(rng, 1000), intn(rng, 100), intn(rng, 100))
		},
	}
}
