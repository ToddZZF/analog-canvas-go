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

static GtkStatusIcon *tray_icon = NULL;
static GtkMenu *tray_menu = NULL;

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
  int *slot = malloc(sizeof(int));
  *slot = id;
  g_signal_connect(item, "activate", G_CALLBACK(on_item_activated), slot);
  gtk_widget_set_tooltip_text(item, tooltip);
  gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), item);
  gtk_widget_show(item);
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
