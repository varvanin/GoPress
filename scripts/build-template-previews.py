"""Build standalone vector illustrations of the three bundled theme layouts."""
from pathlib import Path
from html import escape
import xml.etree.ElementTree as ET
from base64 import b64encode
out=Path(__file__).resolve().parents[1]/'internal/httpui/assets'
def text(x,y,value,size=20,fill='#29251e',font='Arial',weight='400'):
 return f'<text x="{x}" y="{y}" font-family="{font}" font-size="{size}" font-weight="{weight}" fill="{fill}">{escape(value)}</text>'
def line(x,y,w,color): return f'<path d="M{x} {y}h{w}" stroke="{color}"/>'
def rect(x,y,w,h,fill,r=0):return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"/>'
def svg(name,parts):
 data='<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800">'+''.join(parts)+'</svg>'
 ET.fromstring(data)
 assert 'foreignObject' not in data and '<script' not in data
 (out/f'template-{name}.svg').write_text(data)
p=[rect(0,0,1280,800,'#fbfbf9'),rect(56,52,38,38,'none',3),'<rect x="56" y="52" width="38" height="38" rx="3" fill="none" stroke="#20251f"/>',text(68,78,'Т',18,fill='#20251f'),text(106,76,'Тихие заметки',19,fill='#20251f',weight='600'),text(56,127,'Записи, идеи и наблюдения.',12,fill='#646c61'),text(56,149,'Место для главного.',12,fill='#646c61'),line(275,52,0,'#dfe2d9'),'<path d="M275 52v340" stroke="#dfe2d9"/>',text(56,219,'Записи',13,fill='#92513c',weight='600'),text(56,256,'О журнале',13,fill='#20251f'),line(56,300,192,'#dfe2d9'),text(56,337,'⌕  Поиск по сайту',12,fill='#646c61'),text(56,370,'↗  Читать через RSS',12,fill='#646c61'),text(356,65,'ЛИЧНЫЙ ЖУРНАЛ',11,fill='#92513c'),text(352,144,'Пространство',58,fill='#20251f',weight='600'),text(352,208,'для мыслей.',58,fill='#20251f',weight='600'),text(356,260,'Наблюдения, открытия и опыт, которым хочется поделиться.',16,fill='#646c61'),text(356,291,'Без спешки и лишнего шума.',16,fill='#646c61'),text(356,362,'Последние записи',13,fill='#20251f',weight='600'),text(1144,362,'10 записей',11,fill='#646c61'),line(356,381,868,'#20251f'),text(356,416,'Все записи     Дизайн     Культура     Практика',11,fill='#646c61'),line(356,442,868,'#dfe2d9')]
for y,date,title,description in [(490,'27.10.2026','Искусство делать паузу','Почему свободное время — часть творческого процесса.'),(630,'26.10.2026','Красота простых вещей','Ясность начинается с внимания к привычным деталям.'),(770,'25.10.2026','Как сохранить любопытство','')]:
 p += [text(356,y,date,12,fill='#646c61'),text(356,y+25,'Наблюдения',11,fill='#92513c'),text(496,y+4,title,29,fill='#20251f'),text(1196,y+2,'→',20,fill='#646c61'),text(496,y+43,description,14,fill='#646c61'),line(356,y+91,868,'#dfe2d9')]
svg('minimal',p)
p=[rect(0,0,1280,800,'#151718'),text(54,61,'✳',42,fill='#ffb86b'),text(108,58,'Смена перспективы',25,fill='#f1f0e9',weight='600'),text(898,54,'Проекты      О нас      Поиск ↗',14,fill='#f1f0e9'),line(54,91,1172,'#36393a'),text(54,135,'●  НЕЗАВИСИМОЕ ПРОСТРАНСТВО ИДЕЙ',12,fill='#b9b9b0'),text(47,252,'Идеи. Люди.',137,fill='#f1f0e9',weight='600'),text(47,380,'Результат.',137,fill='#ffb86b',weight='600'),text(56,428,'Исследуем новое и делимся тем, что получается.',17,fill='#b9b9b0'),rect(1010,397,216,51,'#ffb86b',26),text(1032,429,'Смотреть проекты ↓',15),line(54,486,1172,'#36393a'),text(54,524,'Избранное',22,fill='#f1f0e9'),text(1046,522,'Всё     Дизайн',13,fill='#b9b9b0')]
for x,bg,title in [(54,'#bfa68b','Форма и пространство'),(654,'#536b70','Новый способ видеть')]:
 p += [rect(x,550,572,186,bg,8),rect(x+155,576,240,130,'#202b2d',4),f'<circle cx="{x+290}" cy="640" r="52" fill="#ffb86b"/>',rect(x+160,628,110,86,'#e4dbc7',2),text(x,775,title,25,fill='#f1f0e9')]
svg('studio',p)
photo=b64encode((out/'journal-cover-1.webp').read_bytes()).decode()
p=[rect(0,0,1280,800,'#f8f7f3'),text(56,65,'Форма и смысл',34,font='Georgia'),text(827,61,'Журнал    О журнале     Поиск    RSS',13,fill='#626b5b'),line(56,106,1168,'#dddfd4'),text(56,157,'●  НЕЗАВИСИМЫЙ ЖУРНАЛ',11,fill='#626b5b'),text(54,218,'Замечать больше.',52,font='Georgia'),text(54,275,'Делиться главным.',52,font='Georgia'),text(855,184,'О дизайне, культуре и внимании',15,fill='#626b5b'),text(855,212,'к повседневному. Идеи, которые',15,fill='#626b5b'),text(855,240,'стоит сохранить.',15,fill='#626b5b'),text(855,281,'Открыть журнал  →',13),line(56,325,1168,'#dddfd4'),text(56,361,'Свежий взгляд',23,font='Georgia'),text(239,358,'10 записей',12,fill='#626b5b'),text(880,358,'Все записи    Дизайн    Культура',12,fill='#626b5b'),rect(56,392,1168,408,'#efeee8'),f'<image x="56" y="392" width="681" height="408" preserveAspectRatio="xMidYMid slice" href="data:image/webp;base64,{photo}"/>',rect(80,416,137,29,'#f8f7f3'),text(91,435,'Главная история',11),text(779,460,'ДИЗАЙН    27.10.2026',12,fill='#626b5b'),text(777,526,'Меньше шума.',43,font='Georgia'),text(777,575,'Больше смысла.',43,font='Georgia'),text(779,624,'Как создавать вещи с характером',14,fill='#626b5b'),text(779,650,'и оставлять место для главного.',14,fill='#626b5b'),text(779,714,'Читать историю   →',13)]
svg('journal',p)
