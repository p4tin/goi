#ifndef GOI_DOCKMENU_H
#define GOI_DOCKMENU_H

#define GOI_DOCK_CHECKED 1
#define GOI_DOCK_DISABLED 2
#define GOI_DOCK_SEPARATOR 4
#define GOI_DOCK_SUBMENU 8

typedef struct {
	int parent;
	long long tag;
	int flags;
	char *title;
} GoiDockItem;

int goiDockInstall(void);
void goiDockApply(int n, GoiDockItem *items);

#endif
