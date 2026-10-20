//go:build !notray && (linux || freebsd || netbsd || openbsd)

// The GTK side of the XEmbed tray fallback: a GtkStatusIcon docked into the
// panel's legacy tray and a popup menu whose activations cross back into Go
// by item id. GTK calls all run on the thread that entered xembed_tray_run.
//
// GtkStatusIcon and gtk_menu_popup are deprecated by GTK in favor of
// StatusNotifier, but they are the only GTK API that speaks the legacy
// XEmbed tray protocol this fallback exists for, and they remain fully
// functional in GTK 3.
#include <gtk/gtk.h>
#include <stdlib.h>
#include <string.h>

static GtkStatusIcon *tray_icon = NULL;
static GtkMenu *tray_menu = NULL;

// Menu rows by the id the Go side assigns at creation; updates target them
// from the loop thread only, through the idle queue below.
static GtkWidget **menu_items = NULL;
static int menu_item_count = 0;

// Defined by cgo from the xembedMenuSelected export in tray_xembed.go.
extern void xembedMenuSelected(int id);

static void xembed_popup(GtkMenu *menu, GtkStatusIcon *icon, guint button,
                         guint activate_time) {
  gtk_menu_popup(menu, NULL, NULL, gtk_status_icon_position_menu, icon, button,
                 activate_time);
}

static void on_activate(GtkStatusIcon *icon, gpointer user_data) {
  xembed_popup(tray_menu, icon, 0, gtk_get_current_event_time());
}

static void on_popup_menu(GtkStatusIcon *icon, guint button, guint time,
                          gpointer user_data) {
  xembed_popup(tray_menu, icon, button, time);
}

static void on_item_activated(GtkMenuItem *item, gpointer user_data) {
  xembedMenuSelected(*(int *)user_data);
}

static void append_menu_item(GtkWidget *item, int id) {
  int *slot = malloc(sizeof(int));
  *slot = id;
  g_signal_connect(item, "activate", G_CALLBACK(on_item_activated), slot);
  gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), item);
  gtk_widget_show(item);
  menu_items = realloc(menu_items, (menu_item_count + 1) * sizeof *menu_items);
  menu_items[menu_item_count++] = item;
}

int xembed_tray_init(void) {
  if (gtk_init_check(0, NULL) == FALSE) {
    return 0;
  }
  tray_icon = gtk_status_icon_new();
  if (tray_icon == NULL) {
    return 0;
  }
  tray_menu = GTK_MENU(gtk_menu_new());
  g_signal_connect(tray_icon, "activate", G_CALLBACK(on_activate), NULL);
  g_signal_connect(tray_icon, "popup-menu", G_CALLBACK(on_popup_menu), NULL);
  return 1;
}

int xembed_tray_set_icon_file(const char *path) {
  GError *error = NULL;
  GdkPixbuf *pixbuf = gdk_pixbuf_new_from_file(path, &error);
  if (error != NULL) {
    g_error_free(error);
    return 0;
  }
  gtk_status_icon_set_from_pixbuf(tray_icon, pixbuf);
  g_object_unref(pixbuf);
  gtk_status_icon_set_visible(tray_icon, TRUE);
  return 1;
}

void xembed_tray_set_tooltip(const char *text) {
  gtk_status_icon_set_tooltip_text(tray_icon, text);
}

void xembed_tray_menu_item(int id, const char *label, const char *tooltip) {
  GtkWidget *item = gtk_menu_item_new_with_label(label);
  gtk_widget_set_tooltip_text(item, tooltip);
  append_menu_item(item, id);
}

void xembed_tray_menu_header(int id, const char *label) {
  GtkWidget *item = gtk_menu_item_new_with_label(label);
  gtk_widget_set_sensitive(item, FALSE);
  append_menu_item(item, id);
}

typedef struct {
  int id;
  char *label;
  char *tooltip;
  int visible;
} menu_item_update;

static gboolean apply_menu_item_update(gpointer data) {
  menu_item_update *update = data;
  if (update->id >= 0 && update->id < menu_item_count &&
      menu_items[update->id] != NULL) {
    if (update->label[0] != '\0') {
      gtk_menu_item_set_label(GTK_MENU_ITEM(menu_items[update->id]),
                              update->label);
    }
    gtk_widget_set_tooltip_text(menu_items[update->id],
                                update->tooltip[0] ? update->tooltip : NULL);
    if (update->visible) {
      gtk_widget_show(menu_items[update->id]);
    } else {
      gtk_widget_hide(menu_items[update->id]);
    }
  }
  free(update->label);
  free(update->tooltip);
  free(update);
  return G_SOURCE_REMOVE;
}

void xembed_tray_menu_item_update(int id, const char *label,
                                  const char *tooltip, int visible) {
  menu_item_update *update = malloc(sizeof(menu_item_update));
  update->id = id;
  update->label = strdup(label ? label : "");
  update->tooltip = strdup(tooltip ? tooltip : "");
  update->visible = visible;
  // Callable from any thread: the rewrite is scheduled on the GTK loop,
  // like the quit path below.
  g_idle_add(apply_menu_item_update, update);
}

void xembed_tray_menu_separator(void) {
  GtkWidget *separator = gtk_separator_menu_item_new();
  gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), separator);
  gtk_widget_show(separator);
}

static gboolean xembed_quit_idle(gpointer user_data) {
  gtk_main_quit();
  return G_SOURCE_REMOVE;
}

void xembed_tray_quit(void) {
  // Callable from any thread: the quit itself is scheduled on the GTK loop.
  g_idle_add(xembed_quit_idle, NULL);
}

void xembed_tray_run(void) { gtk_main(); }
