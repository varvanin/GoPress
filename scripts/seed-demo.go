//go:build ignore

// Run with the local server stopped: go run scripts/seed-demo.go --data ./data
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gopress/internal/backup"
	"gopress/internal/httpui"
	"gopress/internal/service"
	"gopress/internal/store"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func para(text string) service.Node {
	return service.Node{Type: "paragraph", Content: []service.Node{{Type: "text", Text: text}}}
}
func heading(text string) service.Node {
	return service.Node{Type: "heading", Attrs: json.RawMessage(`{"level":2}`), Content: []service.Node{{Type: "text", Text: text}}}
}
func doc(title, lead, body string, items []string, media string) string {
	nodes := []service.Node{para(lead), heading(title), para(body)}
	if media != "" {
		attrs, _ := json.Marshal(map[string]string{"src": "/uploads/" + media, "alt": "Авторская геометрическая иллюстрация", "caption": "Иллюстрация для тестового материала"})
		nodes = append(nodes, service.Node{Type: "image", Attrs: attrs})
	}
	list := service.Node{Type: "bulletList"}
	for _, item := range items {
		list.Content = append(list.Content, service.Node{Type: "listItem", Content: []service.Node{para(item)}})
	}
	nodes = append(nodes, list, heading("С чего начать"), para("Выберите один небольшой шаг и проверьте его на практике. Сохраните результат, вернитесь к нему через несколько дней и сравните впечатления. Хорошее решение становится понятнее в использовании, а не только на красивом макете."), service.Node{Type: "blockquote", Content: []service.Node{para("Меньше случайных решений — больше внимания к тому, что действительно важно.")}})
	raw, err := json.Marshal(service.Node{Type: "doc", Content: nodes})
	must(err)
	return string(raw)
}
func artwork(style int) *image.RGBA {
	w, h := 1600, 1000
	if style == 2 {
		w, h = 1000, 1400
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			u, v := float64(x)/float64(w), float64(y)/float64(h)
			grain := float64((x*17+y*31)%13-6) * .55
			var r, g, b float64
			switch style {
			case 0:
				r, g, b = 231+v*9, 235+v*6, 219+v*5
				if math.Hypot(u-.72, v-.31) < .135 {
					r, g, b = 217, 174, 104
				}
				if v > .64+.095*math.Sin(u*6.5) {
					r, g, b = 118, 145, 120
				}
				if v > .79+.08*math.Cos(u*7) {
					r, g, b = 47, 80, 68
				}
			case 1:
				r, g, b = 227-v*15, 216-v*15, 194-v*12
				if u > .14 && u < .39 && v > .26 {
					r, g, b = 163, 124, 89
				}
				if u > .42 && u < .61 && v > .12 {
					r, g, b = 205, 170, 125
				}
				if u > .67 && u < .87 && v > .4 {
					r, g, b = 115, 128, 117
				}
				if v > .84 {
					r, g, b = 188, 157, 118
				}
				if u > .4 && u < .42 && v > .31 {
					r, g, b = 130, 102, 76
				}
			default:
				r, g, b = 39+v*14, 57+v*14, 63+v*14
				if u > .15 && u < .64 && v > .23 && v < .68 {
					r, g, b = 215, 206, 179
				}
				if math.Hypot(u-.64, (v-.42)*1.4) < .22 {
					r, g, b = 223, 145, 91
				}
				if u > .37 && u < .86 && v > .65 && v < .8 {
					r, g, b = 113, 144, 144
				}
			}
			img.SetRGBA(x, y, color.RGBA{uint8(r + grain), uint8(g + grain), uint8(b + grain), 255})
		}
	}
	return img
}

type article struct {
	Title, Slug, Lead, Body, Category string
	Tags, Items                       []string
	Image                             int
}

var articles = []article{
	{"Пространство, в котором легко думать", "prostranstvo-dlya-idey", "Рабочее место влияет на внимание сильнее, чем кажется. Чтобы сосредоточиться, не обязательно покупать новый стол: иногда достаточно убрать лишнее и правильно направить свет.", "Начните с маршрута обычного дня. Где вы читаете, где обсуждаете идеи, куда складываете заметки? У каждой задачи должна быть понятная опора. Мы собрали несколько наблюдений о спокойных пространствах, которые помогают работать без постоянного переключения.", "Дизайн", []string{"Пространство", "Практика"}, []string{"Освободите одну поверхность для текущей задачи.", "Разделите рабочий и общий свет.", "Оставьте материалы, к которым приятно прикасаться."}, 0},
	{"Почему хорошие сайты оставляют место тишине", "mesto-dlya-tishiny", "Пустое пространство — часть композиции. Оно помогает увидеть главное, почувствовать ритм и спокойно прочитать длинный текст.", "Когда каждый блок требует внимания, посетитель перестаёт различать приоритеты. Попробуйте убрать один декоративный элемент, сократить подпись и увеличить расстояние между разными задачами. Интерфейс станет яснее, а содержание — заметнее.", "Дизайн", []string{"Типографика", "Веб"}, []string{"Один экран — одна основная задача.", "Ритм важнее случайных отступов.", "Проверяйте длинные заголовки и узкие экраны."}, 1},
	{"Небольшой путеводитель по городской прогулке", "gorodskaya-progulka", "Знакомый район умеет удивлять, если пройти по нему без привычной спешки. Посмотрите на вывески, отражения и то, как меняется улица утром и вечером.", "Для прогулки не нужен специальный маршрут. Выберите одну деталь: цвет дверей, формы окон или рисунок теней. Такое ограничение помогает замечать связи и возвращаться с наблюдениями, а не только с фотографиями.", "Культура", []string{"Наблюдения", "Пространство"}, []string{"Выберите тихую улицу вместо короткого пути.", "Сделайте три фотографии одной темы.", "Запишите деталь, которую хочется запомнить."}, 2},
	{"Как подготовить материалы для нового сайта", "materialy-dlya-sayta", "Создание сайта начинается с содержания. Когда тексты, фотографии и ответы на основные вопросы готовы заранее, проект движется спокойнее.", "Соберите материалы в одной папке. Отдельно запишите, кому предназначен сайт и какое действие должно стать следующим для посетителя. Не пытайтесь сразу написать идеальный текст: ясный черновик полезнее красивого, но пустого обещания.", "Практика", []string{"Веб", "Процесс"}, []string{"Опишите аудиторию и её основной вопрос.", "Подготовьте реальные фотографии и подписи.", "Укажите контакты, услуги и условия работы."}, 1},
	{"Типографика без сложных правил", "tipografika-bez-slozhnostey", "Читаемый текст не требует десятка шрифтов. Достаточно понятной иерархии, удобной длины строки и нескольких последовательных решений.", "Начните с основного текста, затем подберите размер заголовков. Посмотрите, как выглядят списки, цитаты и ссылки. Особенно полезно проверить страницу на телефоне: там быстро становятся заметны слишком длинные строки и тяжёлые блоки.", "Дизайн", []string{"Типографика", "Практика"}, []string{"Используйте ограниченный набор начертаний.", "Разделяйте абзацы по смыслу.", "Проверяйте контраст в обеих палитрах."}, 2},
	{"От первой заметки до готового проекта", "ot-zametki-k-proektu", "Идеи редко появляются полностью готовыми. Обычно проект начинается с вопроса, пары набросков и желания понять, можно ли сделать привычную вещь удобнее.", "Сначала сформулируйте проблему своими словами. Затем соберите несколько примеров, сделайте простой прототип и покажите его человеку, который не участвовал в обсуждении. Его вопросы помогут увидеть то, что стало незаметным внутри команды.", "Практика", []string{"Процесс", "Наблюдения"}, []string{"Запишите задачу без описания решения.", "Проверьте самый рискованный шаг первым.", "Сохраняйте причины принятых решений."}, 0},
	{"Предметы, которые остаются с нами надолго", "veschi-nadolgo", "Некоторые вещи становятся частью повседневного ритуала: любимая чашка, удобный блокнот, настольная лампа. Их ценность растёт вместе с историей использования.", "Долговечность складывается из материалов, конструкции и отношения владельца. Нам интересны предметы, которые не требуют постоянной замены и не пытаются выглядеть новыми любой ценой. Следы времени иногда делают вещь только выразительнее.", "Культура", []string{"Наблюдения", "Практика"}, []string{"Обратите внимание на ремонтопригодность.", "Выбирайте понятные материалы.", "Покупайте под реальную привычку, а не обещание новой жизни."}, 1},
	{"Что проверить перед публикацией страницы", "pered-publikaciey", "Последняя проверка нужна даже небольшой странице. Несколько минут внимания помогают избежать неработающих ссылок, случайных черновиков и неподписанных изображений.", "Откройте предпросмотр, пройдите по меню и прочитайте текст вслух. Сравните заголовок страницы с тем, что видит человек в поиске. Затем откройте сайт без авторизации и убедитесь, что опубликована именно та версия, которую вы подготовили.", "Практика", []string{"Веб", "Процесс"}, []string{"Проверьте адрес, заголовок и описание.", "Добавьте альтернативные подписи изображениям.", "Проверьте мобильную навигацию и публичную версию."}, 0},
}

func main() {
	data := flag.String("data", "", "existing local data directory")
	fixture := flag.Bool("fixture", false, "create an isolated test installation")
	flag.Parse()
	if *data == "" {
		log.Fatal("--data is required")
	}
	ctx := context.Background()
	if *fixture {
		if _, err := os.Stat(*data); !os.IsNotExist(err) {
			log.Fatal("fixture destination must not exist")
		}
		s, err := store.Open(*data)
		must(err)
		v, err := service.New(s)
		must(err)
		key := make([]byte, 24)
		_, err = rand.Read(key)
		must(err)
		must(v.Install(ctx, v.InstallToken, store.Site{Title: "Demo test", BaseURL: "http://example.test", Timezone: "Europe/Warsaw"}, "fixture@example.test", fmt.Sprintf("%x", key)))
		must(s.Close())
	}
	abs, err := filepath.Abs(*data)
	must(err)
	dest := filepath.Join(filepath.Dir(abs), "backups", "before-demo-"+time.Now().Format("20060102-150405.000000000"))
	must(os.MkdirAll(filepath.Dir(dest), 0700))
	must(backup.Create(abs, dest))
	must(backup.Verify(dest))
	fmt.Println("Verified backup:", dest)
	s, err := store.Open(abs)
	must(err)
	defer s.Close()
	v, err := service.New(s)
	must(err)
	var owner int64
	must(s.DB.QueryRow("SELECT id FROM users WHERE role='admin' AND blocked=0 ORDER BY id LIMIT 1").Scan(&owner))
	actor, err := s.User(owner)
	must(err)
	ctx = store.WithActor(ctx, actor)
	var images []string
	manifest := filepath.Join(abs, "demo-media.json")
	if raw, err := os.ReadFile(manifest); err == nil {
		must(json.Unmarshal(raw, &images))
	} else if !os.IsNotExist(err) {
		must(err)
	}
	if len(images) == 0 {
		titles := []string{"Тихий ландшафт", "Архитектура света", "Форма и равновесие"}
		for i, title := range titles {
			var buf bytes.Buffer
			if i == 1 {
				must(jpeg.Encode(&buf, artwork(i), &jpeg.Options{Quality: 90}))
			} else {
				must(png.Encode(&buf, artwork(i)))
			}
			name, err := v.Upload(bytes.NewReader(buf.Bytes()), ctx)
			must(err)
			f, err := s.MediaFile(name)
			must(err)
			f.Title = title
			f.Alt = title + ": абстрактная геометрическая композиция"
			f.Caption = "Оригинальная иллюстрация для демонстрационного наполнения GoPress"
			must(v.SaveMediaDetails(f))
			images = append(images, name)
		}
		raw, err := json.MarshalIndent(images, "", "  ")
		must(err)
		must(os.WriteFile(manifest, raw, 0600))
	}
	if len(images) != 3 {
		log.Fatal("invalid demo media manifest")
	}
	for _, name := range images {
		_, err := s.MediaFile(name)
		must(err)
	}
	termIDs := map[string]int64{}
	for kind, names := range map[string][]string{"category": {"Дизайн", "Культура", "Практика"}, "tag": {"Типографика", "Пространство", "Веб", "Наблюдения", "Процесс", "Практика"}} {
		for _, name := range names {
			terms, err := s.Terms(false)
			must(err)
			var id int64
			for _, t := range terms {
				if t.Kind == kind && t.Name == name {
					id = t.ID
				}
			}
			if id == 0 {
				must(v.SaveTerm(ctx, store.Term{Kind: kind, Name: name}, false))
				terms, err = s.Terms(false)
				must(err)
				for _, t := range terms {
					if t.Kind == kind && t.Name == name {
						id = t.ID
					}
				}
			}
			if id == 0 {
				log.Fatal("missing term")
			}
			termIDs[kind+":"+name] = id
		}
	}
	created := 0
	published := []store.Page{}
	save := func(title, slug, kind, lead, body string, items []string, cover int, parent int64, terms []int64, publish bool) store.Page {
		slug = "demo-" + slug
		var id int64
		lookup := slug
		if parent > 0 {
			ancestor, err := s.Page(parent)
			must(err)
			lookup = ancestor.Slug + "/" + slug
		}
		err := s.DB.QueryRow("SELECT id FROM pages WHERE slug=? OR published_slug=? LIMIT 1", lookup, lookup).Scan(&id)
		if err == nil {
			p, err := s.Page(id)
			must(err)
			return p
		}
		if !errors.Is(err, sql.ErrNoRows) {
			must(err)
		}
		p, err := v.Save(ctx, store.Page{Title: title, Slug: slug, Kind: kind, Document: doc(title, lead, body, items, images[cover]), Cover: images[cover], ParentID: parent, TermIDs: terms, SEO: store.SEO{Title: title, Description: lead}}, publish)
		must(err)
		created++
		if publish {
			published = append(published, p)
		}
		return p
	}
	about := save("О студии", "o-studii", "page", "Мы — небольшая независимая студия. Создаём понятные сайты и визуальные системы для людей, которым важны содержание и внимание к деталям.", "Этот раздел показывает, как на сайте могут сочетаться рассказ о команде, описание подхода и фотографии. Все материалы добавлены для локального тестирования; контакты ниже демонстрационные.", []string{"Начинаем с вопросов и исследования.", "Обсуждаем решения понятным языком.", "Проверяем результат на реальных сценариях."}, 0, 0, nil, true)
	save("Наш подход", "podhod", "page", "Мы начинаем проект с задачи, а не с набора эффектов. Сначала разбираемся, кому нужен сайт и какую работу он должен выполнять.", "Эта вложенная страница помогает проверить родительские адреса и навигацию. В хорошем процессе исследование, прототип и содержание развиваются вместе, а важные договорённости фиксируются по ходу работы.", []string{"Знакомство и формулировка задачи.", "Прототип, тексты и визуальное направление.", "Разработка, проверка и запуск."}, 1, about.ID, nil, true)
	save("Услуги", "uslugi", "page", "Помогаем собрать цельный цифровой образ: от структуры небольшого сайта до аккуратной редакционной системы.", "Формат работы зависит от задачи. Небольшой проект можно начать с консультации и прототипа. Для более сложного сайта сначала готовим структуру, сценарии и требования к содержанию.", []string{"Сайты компаний и независимых специалистов.", "Редакционные проекты и портфолио.", "Поддержка, аудит и развитие существующего сайта."}, 2, 0, nil, true)
	portfolio := save("Проекты", "proekty", "page", "Подборка демонстрационных проектов: пространство для идей, журнал наблюдений и сайт небольшой творческой команды.", "Каждый пример показывает сочетание текста, изображения и понятной навигации. Это тестовое портфолио, а не утверждение о выполненных коммерческих заказах.", []string{"Тихий дом — визуальная история пространства.", "Город рядом — журнал повседневных наблюдений.", "Мастерская — представление команды и услуг."}, 1, 0, nil, true)
	save("Тихий дом", "tihiy-dom", "page", "Демонстрационный проект о спокойном пространстве и повседневных привычках. Большие изображения сочетаются с короткими заметками и ясной структурой.", "Для этого примера важны естественные оттенки, мягкий ритм и честное описание задачи. Страница вложена в раздел проектов и подходит для проверки длинного адреса и родительской связи.", []string{"Нейтральная палитра и выразительная типографика.", "Изображения с альтернативными подписями.", "Удобное чтение на широком и узком экране."}, 0, portfolio.ID, nil, true)
	save("Контакты", "kontakty", "page", "Расскажите о своей задаче: какой сайт вы хотите сделать, для кого он предназначен и какой результат ожидаете.", "Демонстрационный адрес для переписки: hello@example.test. Домен .test используется только для примера и не предназначен для отправки реальных сообщений. На этом месте можно разместить форму из конструктора GoPress.", []string{"Коротко опишите проект.", "Укажите удобный способ связи.", "Приложите ссылки на материалы или примеры."}, 2, 0, nil, true)
	for _, a := range articles {
		ids := []int64{termIDs["category:"+a.Category]}
		for _, tag := range a.Tags {
			ids = append(ids, termIDs["tag:"+tag])
		}
		save(a.Title, a.Slug, "post", a.Lead, a.Body, a.Items, a.Image, 0, ids, true)
	}
	draft := save("Черновик: заметки о новом сезоне", "zametki-o-sezone", "post", "Этот материал пока не опубликован. Он нужен для проверки сохранения, предпросмотра и доступа без авторизации.", "Добавьте сюда свои наблюдения, попробуйте изменить обложку и сравните рабочую версию с предпросмотром. Публичный адрес до публикации должен возвращать 404.", []string{"Сохраните изменения.", "Откройте предпросмотр.", "Проверьте адрес в приватном окне."}, 1, 0, []int64{termIDs["category:Культура"]}, false)
	save("Черновик: очень длинный заголовок для проверки переноса строк в списке материалов и на небольшом экране", "dlinnyy-zagolovok", "post", "Материал для проверки длинных заголовков и короткого основного текста.", "Посмотрите на список записей, панель редактора и предпросмотр на телефоне.", []string{"Перенос заголовка.", "Расположение действий.", "Отсутствие горизонтальной прокрутки."}, 2, 0, nil, false)
	future := save("Скоро: маленькие привычки большой команды", "budushchaya-publikaciya", "post", "Тестовая отложенная публикация. Материал запланирован на неделю вперёд.", "Эта запись нужна для проверки списка запланированных публикаций и поведения после перезапуска. При желании измените время публикации в редакторе.", []string{"Еженедельный обзор.", "Короткая встреча по содержанию.", "Один небольшой эксперимент."}, 0, 0, []int64{termIDs["category:Практика"]}, false)
	if future.ScheduledAt == 0 && !future.Published {
		site, err := s.Site()
		must(err)
		loc, err := time.LoadLocation(site.Timezone)
		must(err)
		must(v.Schedule(ctx, future.ID, future.Version, time.Now().Add(7*24*time.Hour).In(loc).Format("2006-01-02T15:04")))
	}
	trash := save("В корзине: пробный материал", "probnyy-material", "post", "Этот материал находится в корзине и нужен для проверки восстановления.", "Восстановите запись через список записей или удалите её навсегда. Это специально созданный тестовый материал.", []string{"Выбор нескольких записей.", "Восстановление из корзины.", "Окончательное удаление."}, 2, 0, nil, false)
	if trash.TrashedAt == 0 {
		must(s.ChangeContent(ctx, trash.ID, trash.Version, "trash"))
	}
	// A published item keeps a private working revision for publication-isolation testing.
	if len(published) > 6 {
		p := published[6]
		p.Title += " — рабочая версия"
		p.Document = doc("Неопубликованные изменения", "Этого текста не должно быть на публичной странице до явного обновления.", "Сравните редактор, предпросмотр и опубликованный адрес. Сохранение черновика не должно менять уже опубликованный материал.", []string{"Рабочая версия.", "Опубликованная версия.", "Явное обновление."}, images[2])
		_, err = v.Save(ctx, p, false)
		must(err)
	}
	for position, items := range map[string][]store.MenuItem{
		"header": {{Label: "О студии", URL: "/" + about.Slug}, {Label: "Наш подход", URL: "/" + about.Slug + "/demo-podhod", Depth: 1}, {Label: "Услуги", URL: "/demo-uslugi"}, {Label: "Проекты", URL: "/" + portfolio.Slug}, {Label: "Тихий дом", URL: "/" + portfolio.Slug + "/demo-tihiy-dom", Depth: 1}, {Label: "Контакты", URL: "/demo-kontakty"}},
		"footer": {{Label: "О студии", URL: "/" + about.Slug}, {Label: "Контакты", URL: "/demo-kontakty"}, {Label: "Поиск", URL: "/search"}},
	} {
		m, err := s.Menu(position)
		must(err)
		for _, item := range items {
			exists := false
			for _, old := range m.Items {
				if old.URL == item.URL {
					exists = true
				}
			}
			if !exists {
				m.Items = append(m.Items, item)
			}
		}
		must(v.SaveMenu(ctx, m))
	}
	app := httpui.New(v)
	for _, p := range published {
		r := httptest.NewRequest("GET", "/"+p.Slug, nil)
		w := httptest.NewRecorder()
		app.Echo.ServeHTTP(w, r)
		if w.Code != 200 {
			log.Fatalf("public check %s: %d", p.Slug, w.Code)
		}
	}
	for _, name := range images {
		w := httptest.NewRecorder()
		app.Echo.ServeHTTP(w, httptest.NewRequest("GET", "/uploads/"+name, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			log.Fatal("image check failed")
		}
		_, _, err := image.Decode(bytes.NewReader(w.Body.Bytes()))
		must(err)
	}
	w := httptest.NewRecorder()
	app.Echo.ServeHTTP(w, httptest.NewRequest("GET", "/"+draft.Slug, nil))
	if w.Code != 404 {
		log.Fatal("draft leaked")
	}
	var integrity string
	must(s.DB.QueryRow("PRAGMA integrity_check").Scan(&integrity))
	if integrity != "ok" {
		log.Fatal(integrity)
	}
	report := map[string]any{"created": created, "images": images, "backup": dest, "draft": draft.Slug, "scheduled": future.Slug, "verified_public_pages": len(published), "verified_images": len(images)}
	raw, err := json.MarshalIndent(report, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(abs, "demo-content-report.json"), raw, 0600))
	_, err = io.Copy(os.Stdout, bytes.NewReader(append(raw, '\n')))
	must(err)
}
