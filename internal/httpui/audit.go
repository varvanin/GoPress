package httpui

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gopress/internal/i18n"
	"gopress/internal/store"
)

func auditDate(ctx context.Context, at int64, zone string) string {
	location, err := time.LoadLocation(zone)
	if err != nil {
		location = time.UTC
	}
	return time.Unix(at, 0).In(location).Format("02.01.2006 15:04:05 MST")
}
func auditActor(ctx context.Context, e store.AuditEvent) string {
	if e.Email != "" {
		return e.Email
	}
	if e.UserID > 0 {
		return i18n.T(ctx, "Пользователь") + " #" + strconv.FormatInt(e.UserID, 10)
	}
	if e.Action == "user.login_failed" {
		return i18n.T(ctx, "Без входа")
	}
	return i18n.T(ctx, "Система")
}
func auditAction(ctx context.Context, action string) string {
	labels := map[string]string{
		"content.schedule_failed":        "Публикация по расписанию не выполнена: проверьте родителя и адрес материала",
		"POST /admin/themes/editor/save": "Редактирование файла шаблона", "POST /admin/themes/editor/copy": "Создание копии шаблона",
		"POST /admin/modules/upload": "Установка модуля", "POST /admin/modules/package/save": "Сохранение настроек модуля", "POST /admin/modules/package/toggle": "Переключение модуля", "POST /admin/modules/package/delete": "Удаление модуля",
		"POST /admin/modules/faq/toggle": "Настройка модуля FAQ", "POST /admin/modules/faq/save": "Сохранение группы FAQ", "POST /admin/modules/faq/delete": "Удаление группы FAQ",
		"user.login": "Вход в админку", "user.login_failed": "Неудачный вход", "user.save": "Изменение пользователя", "user.password": "Смена пароля", "user.reset": "Восстановление пароля", "cli.reset_password": "Восстановление пароля через CLI", "user.reset_requested": "Отправлена ссылка восстановления", "user.reset_delivery_failed": "Ошибка отправки ссылки восстановления",
		"content.move": "Изменение адресов страниц", "content.scheduled": "Публикация по расписанию", "content.create": "Создание материала", "content.update": "Сохранение материала", "content.publish": "Публикация материала", "content.trash": "Перенос в корзину", "content.restore": "Восстановление материала", "content.delete": "Окончательное удаление материала", "content.unpublish": "Снятие с публикации", "content.cancel_schedule": "Отмена публикации по расписанию", "forms.delete_entries": "Удаление заявок", "forms.export": "Экспорт заявок",
		"POST /admin/tools/backups/create": "Создание резервной копии", "POST /admin/tools/backups/restore": "Восстановление сайта", "POST /admin/tools/backups/verify": "Проверка резервной копии", "POST /admin/tools/backups/delete": "Удаление резервной копии", "POST /admin/tools/backups/import": "Импорт резервной копии", "POST /admin/tools/maintenance": "Изменение режима обслуживания", "POST /admin/tools/retention": "Изменение срока хранения журнала", "POST /admin/themes": "Активация шаблона", "POST /admin/appearance": "Активация шаблона", "POST /admin/themes/upload": "Установка шаблона", "POST /admin/themes/delete": "Удаление шаблона", "POST /admin/pages/:id/schedule": "Планирование публикации", "POST /admin/forms/save": "Сохранение формы", "POST /admin/forms/:form/delete": "Удаление формы", "POST /admin/logout": "Выход из админки",
	}
	if label, ok := labels[action]; ok {
		return i18n.T(ctx, label)
	}
	for _, item := range []struct{ prefix, label string }{{"POST /admin/settings/", "Изменение настроек сайта"}, {"POST /admin/media", "Изменение медиатеки"}, {"POST /admin/menus", "Сохранение меню"}, {"POST /admin/posts/terms", "Изменение категорий и тегов"}, {"POST /admin/seo", "Изменение SEO"}, {"POST /admin/forms/", "Изменение формы"}} {
		if strings.HasPrefix(action, item.prefix) {
			return i18n.T(ctx, item.label)
		}
	}
	return i18n.T(ctx, "Действие в админке")
}
func auditObject(ctx context.Context, e store.AuditEvent) string {
	var target struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
		Kind  string `json:"kind"`
	}
	if json.Unmarshal([]byte(e.Object), &target) == nil && target.Title != "" {
		return target.Title + " · #" + strconv.FormatInt(target.ID, 10)
	}
	if id, err := strconv.ParseInt(e.Object, 10, 64); err == nil && id > 0 {
		label := "Материал"
		if strings.HasPrefix(e.Action, "user.") || e.Action == "cli.reset_password" {
			label = "Пользователь"
		} else if strings.Contains(e.Action, "forms") {
			label = "Форма"
		}
		return i18n.T(ctx, label) + " #" + e.Object
	}
	if e.Object != "" {
		return e.Object
	}
	if strings.Contains(e.Action, "/settings") || strings.Contains(e.Action, "/tools/") || strings.Contains(e.Action, "/themes") {
		return i18n.T(ctx, "Сайт")
	}
	return "—"
}
