/**
 * Client-side i18n: translation maps (en, es, ar). Arabic triggers RTL on <html dir="rtl">.
 * Use I18n.t('key') in scripts; use data-i18n / data-i18n-placeholder / data-i18n-aria-label in HTML.
 */
(function (w) {
    'use strict';

    function merge(base, patch) {
        var o = {};
        var k;
        for (k in base) {
            if (Object.prototype.hasOwnProperty.call(base, k)) o[k] = base[k];
        }
        for (k in patch) {
            if (Object.prototype.hasOwnProperty.call(patch, k)) o[k] = patch[k];
        }
        return o;
    }

    var EN = {
        nav_home: 'Home',
        nav_blocks: 'Blocks',
        nav_transactions: 'Transactions',
        nav_addresses: 'Addresses',
        nav_symbols: 'Symbols',
        nav_portfolios: 'Portfolios',
        nav_dashboard: 'Dashboard',
        nav_profile: 'Profile',
        nav_admin: 'Admin',
        brand_subtitle: 'Blockchain Explorer',
        aria_homepage: 'Homepage - Blockchain Explorer',
        aria_main_nav: 'Main navigation',
        aria_mobile_nav: 'Mobile navigation',
        aria_mobile_menu: 'Mobile main menu',
        aria_open_main_menu: 'Open main menu',
        aria_search: 'Search',
        sr_search_blockchain: 'Search blockchain',
        aria_search_main: 'Search by block height, transaction hash, or address',
        aria_search_mobile: 'Mobile search',
        aria_select_theme: 'Select theme',
        theme_blue: 'Blue',
        theme_green: 'Green',
        theme_purple: 'Purple',
        theme_orange: 'Orange',
        search_placeholder_desktop: 'Search by block height, transaction hash, or address…',
        search_placeholder_mobile: 'Search blocks, txs, addresses',
        search_placeholder_bitcoin: 'Search block, transaction, or address…',
        btn_search: 'Search',
        btn_go: 'Go',
        btn_login: 'Login',
        btn_register: 'Register',
        btn_logout: 'Logout',
        home_welcome_prefix: 'Welcome,',
        home_welcome_suffix: '!',
        home_login_failed: 'Login failed',
        home_register_ok: 'Registration successful, please login',
        home_register_fail: 'Registration failed',
        err_api_failure: 'Blockchain search got the hiccups. Try again later.',
        err_unknown_result: 'Unknown result type',
        err_network: 'Network connection issue. Please check your internet.',
        err_invalid_search: 'Invalid search format',
        err_empty_search: 'Please enter a search term',
        feedback_network_error: 'Network error. Please try again later.',
        portfolio_signin_prompt: 'Sign in to view and export your portfolios.',
        portfolio_empty: 'No portfolios found.',
        portfolio_load_fail: 'Failed to load portfolios. Sign in and try again.',
        portfolio_exporting: 'Exporting…',
        portfolio_export_label: 'Export',
        portfolio_export_fail: 'Export failed. Sign in and try again.',
        portfolio_modal_create: 'Create Portfolio',
        portfolio_modal_edit: 'Edit Portfolio',
        portfolio_save_fail: 'Failed to save portfolio',
        portfolio_delete_fail: 'Failed to delete portfolio',
        portfolio_confirm_delete: 'Are you sure you want to delete this portfolio?',
        portfolio_placeholder_label: 'Label',
        portfolio_placeholder_address: 'Address',
        script_no_query: 'No search query provided. Use the search in the header to look up a block, tx or address.',
        script_unknown_result: 'Unknown result type returned by API.',
        script_timeout: 'Request timed out. Please check your connection and try again.',
        script_history_empty: 'No recent views',
        script_share_copied: 'URL copied to clipboard!',
        script_copy: 'Copy',
        script_coinbase: 'Coinbase (New Coins)',
        script_na: 'N/A',
        script_no_tx_found: 'No transactions found',
        script_no_tx_block: 'No transactions',
        script_watchlist_none: 'No watchlists. Create one in Profile.',
        script_watchlist_load_fail: 'Failed to load watchlists.',
        script_watchlist_added: 'Added to watchlist.',
        script_watchlist_add_fail: 'Failed to add.',
        script_watchlist_request_fail: 'Request failed.',
        script_loading: 'Loading data…',
        script_reg_ok: 'Registration successful! Please login.',
        script_admin_cache_none: 'No cached data found',
        script_admin_cache_stats_fail: 'Failed to load cache stats',
        script_admin_cache_cleared: 'Cache cleared successfully! {{count}} keys removed.',
        script_admin_cache_clear_fail: 'Failed to clear cache: {{detail}}',
        script_admin_network: 'Network error occurred',
        script_clear_cache_confirm: 'Are you sure you want to clear all cache? This action cannot be undone.',
        script_chart_metrics_fail: 'Failed to load chart library:',
        script_chart_data_fail: 'Failed to load charts:',
        script_chart_dashboard_fail: 'Failed to load chart library:',
        autocomplete_list: 'Search suggestions',
        autocomplete_count_one: '1 suggestion',
        autocomplete_count_many: '{{n}} suggestions',
        degraded_heading: 'Reduced availability:',
        degraded_suffix: ' Log in, portfolios, and other features that need the database may not work until this is resolved.',
        degraded_default: 'Service dependencies are not ready.',
        degraded_verify: 'Could not verify readiness. The app may be degraded.',
        btn_dismiss: 'Dismiss',
        profile_save_fail: 'Failed to save',
        profile_network: 'Network error. Try again.',
        profile_alerts_loading: 'Loading…',
        profile_alerts_load_fail: 'Failed to load alerts.',
        profile_alert_on: 'On',
        profile_alert_off: 'Off',
        profile_alert_create_fail: 'Failed to create alert',
        profile_alert_create_network: 'Network error creating alert.',
        profile_alert_update_fail: 'Failed to update alert',
        profile_alert_update_network: 'Network error updating alert.',
        profile_alert_delete_fail: 'Failed to delete alert',
        profile_alert_delete_network: 'Network error deleting alert.',
        dashboard_watchlist_signin: 'Sign in to use watchlists',
        dashboard_watchlist_empty_option: 'No watchlists',
        dashboard_watchlist_empty_msg: 'No watchlists',
        dashboard_watchlist_load_fail: 'Error loading watchlists',
        dashboard_watchlist_fail: 'Failed to load watchlist.',
        dashboard_watchlist_entries_fail: 'Failed to load entries.',
        dashboard_watchlist_no_entries: 'No entries. Add addresses or symbols from the explorer.',
        dashboard_watchlist_no_entries_short: 'No entries.',
        dashboard_news_fail: 'Failed to load news. Please try again.',
        dashboard_notif_fail: 'Failed to load notifications.',
        symbols_fetch_fail: 'Failed to fetch symbols: {{detail}}',
        symbols_page_info: 'Page {{current}} of {{total}}',
        symbols_exporting: 'Exporting…',
        symbols_export_json_label: 'Export results (JSON)',
        symbols_export_fail: 'Export failed.',
        symbols_news_fail: 'Failed to load news. Please try again.',
        ui_retry: 'Retry',
        chart_mempool: 'Mempool Size',
        chart_block_time: 'Block Time (s)',
        chart_tx_volume: 'Transaction Volume',
        chart_blockchain_metrics: 'Blockchain Metrics',
        metrics_mempool_title: 'Mempool Size',
        metrics_block_time_title: 'Block Time',
        metrics_tx_volume_title: 'Transaction Volume',
        home_hero_title: 'Explore the Bitcoin Blockchain',
        home_hero_subtitle: 'Search blocks, transactions, and addresses',
        portfolio_page_heading: 'Your Portfolios',
        portfolio_value_in: 'Value in',
        portfolio_create_new: 'Create New Portfolio',
        aria_portfolio_currency: 'Valuation currency',
        portfolio_form_name: 'Name',
        portfolio_form_description: 'Description',
        portfolio_form_addresses: 'Addresses',
        portfolio_add_address: '+ Add Address',
        portfolio_cancel: 'Cancel',
        portfolio_save: 'Save',
        auth_heading_login: 'Login',
        auth_heading_register: 'Register',
        auth_label_username: 'Username',
        auth_label_password: 'Password',
        auth_label_email_optional: 'Email (optional)',
        auth_btn_login: 'Login',
        auth_btn_register: 'Register',
        auth_switch_to_register: "Don't have an account? Register",
        auth_switch_to_login: 'Already have an account? Login',
        footer_about_heading: 'About Blockchain Explorer',
        footer_about_body: 'Explore the Bitcoin blockchain with real-time data on blocks, transactions, and addresses.',
        footer_feedback_heading: 'Feedback',
        footer_feedback_intro: 'Help us improve! Share your thoughts and suggestions.',
        footer_label_name_opt: 'Name (optional)',
        footer_label_email_opt: 'Email (optional)',
        footer_label_message: 'Message',
        footer_placeholder_name: 'Your name',
        footer_placeholder_email: 'your@email.com',
        footer_placeholder_message: 'Your feedback…',
        footer_submit: 'Submit Feedback',
    };

    var ES = merge(EN, {
        nav_home: 'Inicio',
        nav_blocks: 'Bloques',
        nav_transactions: 'Transacciones',
        nav_addresses: 'Direcciones',
        nav_symbols: 'Símbolos',
        nav_portfolios: 'Carteras',
        nav_dashboard: 'Panel',
        nav_profile: 'Perfil',
        nav_admin: 'Admin',
        aria_open_main_menu: 'Abrir menú principal',
        sr_search_blockchain: 'Buscar en la cadena de bloques',
        aria_search_main: 'Buscar por altura de bloque, hash de transacción o dirección',
        aria_search_mobile: 'Búsqueda móvil',
        aria_select_theme: 'Seleccionar tema',
        search_placeholder_desktop: 'Buscar por altura de bloque, hash o dirección…',
        search_placeholder_mobile: 'Bloques, transacciones, direcciones',
        search_placeholder_bitcoin: 'Bloque, transacción o dirección…',
        btn_search: 'Buscar',
        btn_go: 'Ir',
        btn_login: 'Iniciar sesión',
        btn_register: 'Registrarse',
        btn_logout: 'Cerrar sesión',
        home_welcome_prefix: 'Bienvenido,',
        home_login_failed: 'Error al iniciar sesión',
        home_register_ok: 'Registro correcto, inicie sesión',
        home_register_fail: 'Error en el registro',
        err_api_failure: 'La búsqueda falló. Inténtelo más tarde.',
        err_unknown_result: 'Tipo de resultado desconocido',
        err_network: 'Problema de red. Compruebe su conexión.',
        err_invalid_search: 'Formato de búsqueda no válido',
        err_empty_search: 'Introduzca un término de búsqueda',
        feedback_network_error: 'Error de red. Inténtelo más tarde.',
        portfolio_signin_prompt: 'Inicie sesión para ver y exportar sus carteras.',
        portfolio_empty: 'No hay carteras.',
        portfolio_load_fail: 'Error al cargar carteras. Inicie sesión e inténtelo de nuevo.',
        portfolio_exporting: 'Exportando…',
        portfolio_export_label: 'Exportar',
        portfolio_export_fail: 'Error al exportar. Inicie sesión e inténtelo de nuevo.',
        portfolio_modal_create: 'Crear cartera',
        portfolio_modal_edit: 'Editar cartera',
        portfolio_save_fail: 'No se pudo guardar la cartera',
        portfolio_delete_fail: 'No se pudo eliminar la cartera',
        portfolio_confirm_delete: '¿Eliminar esta cartera?',
        portfolio_placeholder_label: 'Etiqueta',
        portfolio_placeholder_address: 'Dirección',
        script_no_query: 'Sin consulta. Use la búsqueda del encabezado.',
        script_unknown_result: 'Tipo de resultado desconocido devuelto por la API.',
        script_timeout: 'Tiempo de espera agotado. Compruebe su conexión.',
        script_history_empty: 'Sin vistas recientes',
        script_share_copied: '¡URL copiada al portapapeles!',
        script_copy: 'Copiar',
        script_coinbase: 'Coinbase (monedas nuevas)',
        script_na: 'N/D',
        script_no_tx_found: 'No se encontraron transacciones',
        script_no_tx_block: 'Sin transacciones',
        script_watchlist_none: 'Sin listas. Cree una en Perfil.',
        script_watchlist_load_fail: 'Error al cargar listas.',
        script_watchlist_added: 'Añadido a la lista.',
        script_watchlist_add_fail: 'Error al añadir.',
        script_watchlist_request_fail: 'Error en la solicitud.',
        script_loading: 'Cargando datos…',
        script_reg_ok: '¡Registro correcto! Inicie sesión.',
        script_admin_cache_none: 'Sin datos en caché',
        script_admin_cache_stats_fail: 'Error al cargar estadísticas de caché',
        script_admin_cache_cleared: '¡Caché borrado! Se eliminaron {{count}} claves.',
        script_admin_cache_clear_fail: 'Error al borrar caché: {{detail}}',
        script_admin_network: 'Error de red',
        script_clear_cache_confirm: '¿Borrar toda la caché? Esta acción no se puede deshacer.',
        script_chart_metrics_fail: 'Error al cargar gráficos:',
        script_chart_data_fail: 'Error al cargar gráficos:',
        script_chart_dashboard_fail: 'Error al cargar gráficos:',
        autocomplete_list: 'Sugerencias de búsqueda',
        autocomplete_count_one: '1 sugerencia',
        autocomplete_count_many: '{{n}} sugerencias',
        degraded_heading: 'Disponibilidad reducida:',
        degraded_suffix: ' El inicio de sesión, carteras y otras funciones pueden no funcionar hasta que se resuelva.',
        degraded_default: 'Los servicios no están listos.',
        degraded_verify: 'No se pudo verificar el estado. La aplicación puede estar degradada.',
        btn_dismiss: 'Cerrar',
        profile_save_fail: 'Error al guardar',
        profile_network: 'Error de red. Inténtelo de nuevo.',
        profile_alerts_loading: 'Cargando…',
        profile_alerts_load_fail: 'Error al cargar alertas.',
        profile_alert_on: 'Sí',
        profile_alert_off: 'No',
        profile_alert_create_fail: 'Error al crear alerta',
        profile_alert_create_network: 'Error de red al crear alerta.',
        profile_alert_update_fail: 'Error al actualizar alerta',
        profile_alert_update_network: 'Error de red al actualizar alerta.',
        profile_alert_delete_fail: 'Error al eliminar alerta',
        profile_alert_delete_network: 'Error de red al eliminar alerta.',
        dashboard_watchlist_signin: 'Inicie sesión para usar listas',
        dashboard_watchlist_empty_option: 'Sin listas',
        dashboard_watchlist_empty_msg: 'Sin listas',
        dashboard_watchlist_load_fail: 'Error al cargar listas',
        dashboard_watchlist_fail: 'Error al cargar la lista.',
        dashboard_watchlist_entries_fail: 'Error al cargar entradas.',
        dashboard_watchlist_no_entries: 'Sin entradas. Añada direcciones o símbolos.',
        dashboard_watchlist_no_entries_short: 'Sin entradas.',
        dashboard_news_fail: 'Error al cargar noticias.',
        dashboard_notif_fail: 'Error al cargar notificaciones.',
        symbols_fetch_fail: 'Error al obtener símbolos: {{detail}}',
        symbols_page_info: 'Página {{current}} de {{total}}',
        symbols_exporting: 'Exportando…',
        symbols_export_json_label: 'Exportar resultados (JSON)',
        symbols_export_fail: 'Error al exportar.',
        symbols_news_fail: 'Error al cargar noticias.',
        ui_retry: 'Reintentar',
        chart_mempool: 'Mempool',
        chart_block_time: 'Tiempo de bloque (s)',
        chart_tx_volume: 'Volumen de transacciones',
        chart_blockchain_metrics: 'Métricas de la cadena',
        metrics_mempool_title: 'Tamaño del mempool',
        metrics_block_time_title: 'Tiempo de bloque',
        metrics_tx_volume_title: 'Volumen de transacciones',
        home_hero_title: 'Explorar la cadena de bloques de Bitcoin',
        home_hero_subtitle: 'Buscar bloques, transacciones y direcciones',
        portfolio_page_heading: 'Sus carteras',
        portfolio_value_in: 'Valor en',
        portfolio_create_new: 'Crear cartera',
        aria_portfolio_currency: 'Moneda de valoración',
        portfolio_form_name: 'Nombre',
        portfolio_form_description: 'Descripción',
        portfolio_form_addresses: 'Direcciones',
        portfolio_add_address: '+ Añadir dirección',
        portfolio_cancel: 'Cancelar',
        portfolio_save: 'Guardar',
        auth_heading_login: 'Iniciar sesión',
        auth_heading_register: 'Registrarse',
        auth_label_username: 'Usuario',
        auth_label_password: 'Contraseña',
        auth_label_email_optional: 'Correo (opcional)',
        auth_btn_login: 'Iniciar sesión',
        auth_btn_register: 'Registrarse',
        auth_switch_to_register: '¿No tiene cuenta? Regístrese',
        auth_switch_to_login: '¿Ya tiene cuenta? Inicie sesión',
        footer_about_heading: 'Acerca del explorador',
        footer_about_body: 'Explore la cadena de bloques de Bitcoin con datos en tiempo real.',
        footer_feedback_heading: 'Comentarios',
        footer_feedback_intro: '¡Ayúdenos a mejorar! Comparta sus ideas.',
        footer_label_name_opt: 'Nombre (opcional)',
        footer_label_email_opt: 'Correo (opcional)',
        footer_label_message: 'Mensaje',
        footer_placeholder_name: 'Su nombre',
        footer_placeholder_email: 'su@correo.com',
        footer_placeholder_message: 'Sus comentarios…',
        footer_submit: 'Enviar comentarios',
    });

    var AR = merge(EN, {
        nav_home: 'الرئيسية',
        nav_blocks: 'كتل',
        nav_transactions: 'معاملات',
        nav_addresses: 'عناوين',
        nav_symbols: 'رموز',
        nav_portfolios: 'محافظ',
        nav_dashboard: 'لوحة التحكم',
        nav_profile: 'الملف',
        nav_admin: 'مسؤول',
        aria_open_main_menu: 'فتح القائمة الرئيسية',
        sr_search_blockchain: 'البحث في سلسلة الكتل',
        aria_search_main: 'بحث بالارتفاع أو تجزئة المعاملة أو العنوان',
        aria_search_mobile: 'بحث جوال',
        btn_search: 'بحث',
        btn_go: 'انتقل',
        btn_login: 'تسجيل الدخول',
        btn_register: 'تسجيل',
        btn_logout: 'تسجيل الخروج',
        brand_subtitle: 'مستكشف سلسلة الكتل',
        aria_homepage: 'الصفحة الرئيسية - مستكشف سلسلة الكتل',
        aria_select_theme: 'اختيار المظهر',
        theme_blue: 'أزرق',
        theme_green: 'أخضر',
        theme_purple: 'بنفسجي',
        theme_orange: 'برتقالي',
        home_welcome_prefix: 'مرحباً،',
        home_welcome_suffix: '!',
        search_placeholder_mobile: 'كتل، معاملات، عناوين',
        home_login_failed: 'فشل تسجيل الدخول',
        err_empty_search: 'يرجى إدخال مصطلح بحث',
        err_invalid_search: 'تنسيق بحث غير صالح',
        script_copy: 'نسخ',
        script_na: 'غير متوفر',
        btn_dismiss: 'إغلاق',
        autocomplete_list: 'اقتراحات البحث',
        autocomplete_count_one: 'اقتراح واحد',
        autocomplete_count_many: '{{n}} اقتراحات',
        degraded_heading: 'توفر أقل:',
        degraded_suffix: ' قد لا تعمل بعض الميزات حتى يتم الحل.',
        degraded_default: 'الخدمات غير جاهزة.',
        degraded_verify: 'تعذر التحقق من الجاهزية.',
        portfolio_confirm_delete: 'حذف هذه المحفظة؟',
        script_clear_cache_confirm: 'مسح كل الذاكرة المؤقتة؟ لا يمكن التراجع.',
        home_hero_title: 'استكشف سلسلة كتل بيتكوين',
        home_hero_subtitle: 'ابحث عن الكتل والمعاملات والعناوين',
        portfolio_page_heading: 'محافظك',
        portfolio_value_in: 'القيمة بـ',
        portfolio_create_new: 'إنشاء محفظة',
        aria_portfolio_currency: 'عملة التقييم',
        portfolio_form_name: 'الاسم',
        portfolio_form_description: 'الوصف',
        portfolio_form_addresses: 'العناوين',
        portfolio_add_address: '+ إضافة عنوان',
        portfolio_cancel: 'إلغاء',
        portfolio_save: 'حفظ',
        auth_heading_login: 'تسجيل الدخول',
        auth_heading_register: 'التسجيل',
        auth_label_username: 'اسم المستخدم',
        auth_label_password: 'كلمة المرور',
        auth_label_email_optional: 'البريد (اختياري)',
        auth_btn_login: 'تسجيل الدخول',
        auth_btn_register: 'تسجيل',
        auth_switch_to_register: 'ليس لديك حساب؟ سجّل',
        auth_switch_to_login: 'لديك حساب؟ سجّل الدخول',
        footer_about_heading: 'عن المستكشف',
        footer_about_body: 'استكشف سلسلة كتل بيتكوين ببيانات فورية.',
        footer_feedback_heading: 'ملاحظات',
        footer_feedback_intro: 'ساعدنا على التحسين.',
        footer_label_name_opt: 'الاسم (اختياري)',
        footer_label_email_opt: 'البريد (اختياري)',
        footer_label_message: 'الرسالة',
        footer_placeholder_name: 'اسمك',
        footer_placeholder_email: 'بريدك@',
        footer_placeholder_message: 'ملاحظاتك…',
        footer_submit: 'إرسال الملاحظات',
        ui_retry: 'إعادة المحاولة',
    });

    var STRINGS = { en: EN, es: ES, ar: AR };
    var RTL_LANGS = { ar: true, he: true, fa: true, ur: true };

    function normalizeLang(lang) {
        if (!lang || typeof lang !== 'string') return 'en';
        var code = String(lang).trim().toLowerCase().slice(0, 2);
        if (STRINGS[code]) return code;
        return 'en';
    }

    function interpolate(str, params) {
        if (!params) return str;
        return str.replace(/\{\{(\w+)\}\}/g, function (_, key) {
            return params[key] != null ? String(params[key]) : '';
        });
    }

    function t(key, params) {
        var lang = normalizeLang(
            w.document && w.document.documentElement && w.document.documentElement.getAttribute('lang')
        );
        var table = STRINGS[lang] || STRINGS.en;
        var s = (table && table[key]) || STRINGS.en[key] || key;
        return interpolate(s, params);
    }

    function setLang(lang) {
        var code = normalizeLang(lang || 'en');
        if (w.document && w.document.documentElement) {
            w.document.documentElement.setAttribute('lang', code);
            w.document.documentElement.setAttribute('dir', RTL_LANGS[code] ? 'rtl' : 'ltr');
        }
        try {
            if (w.localStorage) w.localStorage.setItem('blockchain-explorer-ui-lang', code);
        } catch (e) { /* ignore */ }
        applyDataI18n(w.document);
    }

    function getLang() {
        return normalizeLang(
            w.document && w.document.documentElement && w.document.documentElement.getAttribute('lang')
        );
    }

    function applyDataI18n(root) {
        var r = root || w.document;
        if (!r || !r.querySelectorAll) return;
        var i;
        var nodes = r.querySelectorAll('[data-i18n]');
        for (i = 0; i < nodes.length; i++) {
            var el = nodes[i];
            var key = el.getAttribute('data-i18n');
            var attr = el.getAttribute('data-i18n-attr');
            var val = t(key);
            if (attr) el.setAttribute(attr, val);
            else el.textContent = val;
        }
        nodes = r.querySelectorAll('[data-i18n-placeholder]');
        for (i = 0; i < nodes.length; i++) {
            el = nodes[i];
            el.setAttribute('placeholder', t(el.getAttribute('data-i18n-placeholder')));
        }
        nodes = r.querySelectorAll('[data-i18n-aria-label]');
        for (i = 0; i < nodes.length; i++) {
            el = nodes[i];
            el.setAttribute('aria-label', t(el.getAttribute('data-i18n-aria-label')));
        }
        nodes = r.querySelectorAll('[data-i18n-title]');
        for (i = 0; i < nodes.length; i++) {
            el = nodes[i];
            el.setAttribute('title', t(el.getAttribute('data-i18n-title')));
        }
    }

    function initFromStorage() {
        try {
            var stored = w.localStorage && w.localStorage.getItem('blockchain-explorer-ui-lang');
            if (stored && STRINGS[normalizeLang(stored)]) {
                setLang(stored);
                return;
            }
        } catch (e) { /* ignore */ }
        applyDataI18n(w.document);
    }

    w.I18n = { t: t, setLang: setLang, getLang: getLang, applyDataI18n: applyDataI18n, normalizeLang: normalizeLang };

    if (w.document) {
        if (w.document.readyState === 'loading') {
            w.document.addEventListener('DOMContentLoaded', initFromStorage);
        } else {
            initFromStorage();
        }
    }
})(typeof window !== 'undefined' ? window : this);
