package httpui

import (
	"gopress/internal/store"
)

type AttentionItem struct {
	Label, URL string
	Count      int
}

func (a *App) dashboardAttention(data store.DashboardData, themeFailed bool) ([]AttentionItem, error) {
	items := []AttentionItem{}
	if data.FailedMail > 0 {
		items = append(items, AttentionItem{"Не отправлены уведомления о заявках", "/admin/forms", data.FailedMail})
	}
	if data.FailedPublications > 0 {
		items = append(items, AttentionItem{"Не выполнена публикация по расписанию", "/admin/tools?tab=audit", data.FailedPublications})
	}
	if data.MissingHome {
		items = append(items, AttentionItem{"Выбранная главная страница недоступна", "/admin/settings?tab=home", 0})
	}
	if themeFailed {
		items = append(items, AttentionItem{"Активный шаблон недоступен", "/admin/themes", 0})
	}
	schedule, err := a.Service.Store.BackupSchedule()
	if err != nil {
		return nil, err
	}
	if schedule.Enabled && schedule.Failed {
		items = append(items, AttentionItem{"Проверьте автоматическое резервное копирование", "/admin/tools", 0})
	}
	active, err := a.Service.Maintenance()
	if err != nil {
		return nil, err
	}
	if active {
		items = append(items, AttentionItem{"Включён режим обслуживания", "/admin/tools?tab=system", 0})
	}
	update, err := a.Service.UpdateState()
	if err != nil || update.Phase == "failed" || update.Phase == "uncertain" {
		items = append(items, AttentionItem{"Проверьте состояние обновления", "/admin/updates", 0})
	}
	return items, nil
}
