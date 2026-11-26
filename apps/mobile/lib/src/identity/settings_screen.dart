import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:hnuhole_image_viewer/image_viewer.dart';

import '../auth/auth_presentation.dart';
import '../auth/auth_session_controller.dart';
import '../design_system/telegram_theme.dart';
import '../design_system/motion.dart';
import '../localization/app_localizations.dart';

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({
    super.key,
    required this.onManageIdentity,
    required this.onManageSecurity,
    this.onClose,
    this.onLogout,
    this.onManageAccount,
    this.onCloseAccount,
    this.onBlacklist,
    this.onChangePassword,
    this.supportGalleryWriter,
    this.accountId,
    this.username,
    this.sessions,
    this.sessionAuthority,
  });
  final VoidCallback onManageIdentity, onManageSecurity;
  final VoidCallback? onClose,
      onLogout,
      onManageAccount,
      onCloseAccount,
      onBlacklist,
      onChangePassword;
  final ViewerGalleryWriter? supportGalleryWriter;
  final String? accountId, username;
  final AuthSessionController? sessions;
  final int? sessionAuthority;
  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  bool _opening = false;
  bool _retiring = false;
  ModalRoute<dynamic>? _settingsRoute;
  final _ownedPages = <Route<dynamic>>{};

  bool get _hasAuthority {
    final sessions = widget.sessions;
    return sessions == null ||
        (sessions.isAuthenticated &&
            sessions.authorityVersion == widget.sessionAuthority);
  }

  @override
  void initState() {
    super.initState();
    widget.sessions?.addListener(_retireIfStale);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _settingsRoute = ModalRoute.of(context);
    _retireIfStale();
  }

  @override
  void dispose() {
    widget.sessions?.removeListener(_retireIfStale);
    super.dispose();
  }

  void _retireIfStale() {
    if (_hasAuthority || _retiring) return;
    _retiring = true;
    final settings = _settingsRoute;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      for (final route in _ownedPages.toList().reversed) {
        if (route.isActive) route.navigator?.removeRoute(route);
      }
      if (settings != null && settings.isActive && !settings.isFirst) {
        settings.navigator?.removeRoute(settings);
      }
    });
    WidgetsBinding.instance.ensureVisualUpdate();
  }

  Widget _guard(Widget child) {
    final sessions = widget.sessions;
    if (sessions == null) return child;
    final authority = widget.sessionAuthority;
    return ListenableBuilder(
      listenable: sessions,
      builder: (context, _) =>
          sessions.isAuthenticated && sessions.authorityVersion == authority
          ? child
          : const SizedBox.shrink(),
    );
  }

  Future<T?> _push<T>(Route<T> route) async {
    _ownedPages.add(route);
    try {
      return await Navigator.of(context).push(route);
    } finally {
      _ownedPages.remove(route);
    }
  }

  // 子页统一走 HhSlidePageRoute（微信式：从右缘纯位移滑入、覆盖当前页、
  // 无渐变/缩放），与「修改密码/身份管理/黑名单」等 main.dart 推入的
  // 页面完全一致。
  void _open(Widget page) {
    final guarded = _guard(page);
    unawaited(_push(HhSlidePageRoute<void>(builder: (_) => guarded)));
  }

  void _select(_SettingEntry entry) {
    if (!mounted ||
        !_hasAuthority ||
        _opening ||
        ModalRoute.isCurrentOf(context) == false) {
      return;
    }
    _opening = true;
    FocusManager.instance.primaryFocus?.unfocus();
    entry.open();
    WidgetsBinding.instance.addPostFrameCallback((_) => _opening = false);
  }

  Future<void> _confirmLogout() async {
    if (!_hasAuthority) return;
    final confirmed = await _push(
      DialogRoute<bool>(
        context: context,
        builder: (dialogContext) => _guard(
          AlertDialog(
            key: const ValueKey('settings-logout-confirmation'),
            title: Text(hhText(dialogContext, '退出登录')),
            content: Text(hhText(dialogContext, '确定退出登录吗？')),
            actions: [
              TextButton(
                key: const ValueKey('settings-logout-cancel'),
                onPressed: () => Navigator.pop(dialogContext, false),
                child: Text(hhText(dialogContext, '取消')),
              ),
              FilledButton(
                key: const ValueKey('settings-logout-confirm'),
                onPressed: () => Navigator.pop(dialogContext, true),
                child: Text(hhText(dialogContext, '确定')),
              ),
            ],
          ),
        ),
      ),
    );
    if (confirmed == true && mounted && _hasAuthority) widget.onLogout?.call();
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final controller = AuthPresentationScope.maybeOf(context);
    final themeLabel = switch (controller?.themeMode) {
      ThemeMode.light => '浅色',
      ThemeMode.dark => '深色',
      _ => '跟随系统',
    };
    final fontLabel = switch (controller?.fontScale) {
      'small' => '小',
      'large' => '大',
      'xlarge' => '特大',
      _ => '标准',
    };
    Widget page = _SettingsPage(
      title: '设置',
      onBack: widget.onClose,
      child: ColoredBox(
        color: p.background,
        child: ListenableBuilder(
          listenable: controller ?? const ListenableNever(),
          builder: (context, _) => ListView(
            key: const ValueKey('settings-list'),
            // 首个分组标题的上方留白由 _SettingsGroup 自身提供，
            // 避免列表内边距叠加造成首标题上下不对称。
            padding: const EdgeInsets.only(bottom: 24),
            children: [
              _SettingsGroup(
                label: '偏好',
                rows: [
                  _SettingEntry(
                    'theme',
                    Icons.dark_mode_outlined,
                    '主题',
                    value: themeLabel,
                    open: () => _open(const _ThemeSettings()),
                  ),
                  _SettingEntry(
                    'language',
                    Icons.language_outlined,
                    '语言',
                    value: controller?.language.nativeName,
                    open: () => _open(const _LanguageSettings()),
                  ),
                  _SettingEntry(
                    'fontsize',
                    Icons.format_size_outlined,
                    '字体大小',
                    value: fontLabel,
                    open: () => _open(const _FontSizeSettings()),
                  ),
                ],
              ),
              _SettingsGroup(
                label: '账号与安全',
                rows: [
                  _SettingEntry(
                    'account-profile',
                    Icons.badge_outlined,
                    '账号资料',
                    subtitle: '平台编号与用户名',
                    open: () => _open(
                      _AccountProfile(
                        accountId: widget.accountId,
                        username: widget.username,
                      ),
                    ),
                  ),
                  _SettingEntry(
                    'password',
                    Icons.lock_outline_rounded,
                    '修改密码',
                    subtitle: '修改后其他设备需重新登录',
                    // 复用登录页找回账号中的校邮重置密码流程。
                    open:
                        widget.onChangePassword ??
                        () => _open(
                          const _SettingsUnavailable(
                            title: '修改密码',
                            message: '修改密码暂时不可用，请稍后重试。',
                          ),
                        ),
                  ),
                  _SettingEntry(
                    'devices',
                    Icons.devices_outlined,
                    '设备管理',
                    open: widget.onManageSecurity,
                  ),
                  _SettingEntry(
                    'closure',
                    Icons.shield_outlined,
                    '账号注销',
                    subtitle: '七天缓冲期内可撤销',
                    danger: true,
                    open:
                        widget.onCloseAccount ??
                        widget.onManageAccount ??
                        () => _open(
                          const _SettingsUnavailable(
                            title: '账号注销',
                            message: '账号管理暂时不可用，请稍后重试。',
                          ),
                        ),
                  ),
                  if (widget.onLogout == null && widget.onManageAccount != null)
                    _SettingEntry(
                      'account',
                      Icons.person_outline_rounded,
                      '账号与安全',
                      open: widget.onManageAccount!,
                    ),
                ],
              ),
              _SettingsGroup(
                label: '社区与隐私',
                rows: [
                  _SettingEntry(
                    'identity',
                    Icons.switch_account_outlined,
                    '身份管理',
                    subtitle: '昵称、头像与发言身份',
                    open: widget.onManageIdentity,
                  ),
                  _SettingEntry(
                    'blacklist',
                    Icons.block_outlined,
                    '黑名单',
                    open:
                        widget.onBlacklist ??
                        () => _open(
                          const _SettingsUnavailable(
                            title: '黑名单',
                            message: '黑名单暂时不可用，请稍后重试。',
                          ),
                        ),
                  ),
                ],
              ),
              _SettingsGroup(
                label: '支持与关于',
                rows: [
                  _SettingEntry(
                    'feedback',
                    Icons.chat_bubble_outline_rounded,
                    '意见反馈',
                    open: () => _open(const _FeedbackSettings()),
                  ),
                  _SettingEntry(
                    'support',
                    Icons.favorite_border_rounded,
                    '支持我们',
                    open: () => _open(
                      _SupportSettings(gallery: widget.supportGalleryWriter),
                    ),
                  ),
                  _SettingEntry(
                    'about',
                    Icons.info_outline_rounded,
                    '关于 SpeechFUL',
                    value: 'v2.0.0',
                    open: () => _open(const _AboutSettings()),
                  ),
                ],
              ),
              if (widget.onLogout != null) ...[
                const SizedBox(height: 30),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: HhPressFeedback(
                    pressedScale: .985,
                    builder: (context, states) => Material(
                      color: p.error.withValues(alpha: .08),
                      borderRadius: BorderRadius.circular(12),
                      clipBehavior: Clip.antiAlias,
                      child: InkWell(
                        statesController: states,
                        key: const ValueKey('settings-logout'),
                        onTap: () => unawaited(_confirmLogout()),
                        child: SizedBox(
                          width: double.infinity,
                          child: Padding(
                            padding: const EdgeInsets.symmetric(vertical: 14),
                            child: Center(
                              child: Text(
                                hhText(context, '退出登录'),
                                style: TextStyle(
                                  fontSize: 15,
                                  fontWeight: FontWeight.w500,
                                  color: p.error,
                                ),
                              ),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
    );
    // 微信式视差：子页推入时设置列表左移约 30%，返回时滑回。
    final secondary = ModalRoute.of(context)?.secondaryAnimation;
    if (secondary != null) {
      page = SlideTransition(
        position: secondary
            .drive(CurveTween(curve: HhMotion.pageSlideCurve))
            .drive(Tween(begin: Offset.zero, end: const Offset(-.3, 0))),
        child: page,
      );
    }
    return _guard(page);
  }
}

/// An inert fallback so the grouped list keeps rebuilding with appearance.
class ListenableNever implements Listenable {
  const ListenableNever();
  @override
  void addListener(VoidCallback listener) {}
  @override
  void removeListener(VoidCallback listener) {}
}

class _SettingEntry {
  const _SettingEntry(
    this.id,
    this.icon,
    this.title, {
    this.subtitle,
    this.value,
    this.danger = false,
    required this.open,
  });
  final String id, title;
  final String? subtitle, value;
  final bool danger;
  final IconData icon;
  final VoidCallback open;
}

class _SettingsGroup extends StatelessWidget {
  const _SettingsGroup({required this.label, required this.rows});
  final String label;
  final List<_SettingEntry> rows;
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          // 上下等距：标题在上一区域与本组内容之间居中。
          padding: const EdgeInsets.fromLTRB(22, 12, 22, 12),
          child: Text(
            hhText(context, label),
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w600,
              letterSpacing: .4,
              color: p.secondary,
            ),
          ),
        ),
        DecoratedBox(
          decoration: BoxDecoration(
            color: p.surface,
            border: Border(
              top: BorderSide(color: p.line),
              bottom: BorderSide(color: p.line),
            ),
          ),
          child: Column(
            children: [
              for (var i = 0; i < rows.length; i++)
                _SettingsRow(entry: rows[i], showBorder: i < rows.length - 1),
            ],
          ),
        ),
      ],
    );
  }
}

class _SettingsRow extends StatelessWidget {
  const _SettingsRow({required this.entry, this.showBorder = true});
  final _SettingEntry entry;
  final bool showBorder;
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final color = entry.danger ? p.error : p.ink;
    final iconColor = entry.danger ? p.error : p.secondary;
    final subtitle = entry.subtitle;
    final value = entry.value;
    return DecoratedBox(
      decoration: BoxDecoration(
        border: showBorder ? Border(bottom: BorderSide(color: p.line)) : null,
      ),
      child: HhPressFeedback(
        pressedScale: .985,
        builder: (context, states) => InkWell(
          statesController: states,
          key: ValueKey('settings-${entry.id}'),
          onTap: () => _selectRow(context),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
            child: Row(
              children: [
                SizedBox(
                  width: 24,
                  child: Icon(entry.icon, size: 22, color: iconColor),
                ),
                const SizedBox(width: 13),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        hhText(context, entry.title),
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w500,
                          color: color,
                          height: 1.35,
                        ),
                      ),
                      if (subtitle != null) ...[
                        const SizedBox(height: 3),
                        Text(
                          hhText(context, subtitle),
                          style: TextStyle(
                            fontSize: 12,
                            height: 1.5,
                            color: p.secondary,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
                if (value != null) ...[
                  const SizedBox(width: 10),
                  // 普通子项而非 Flexible：标题 Expanded 独占剩余宽度，
                  // 值与 chevron 被推到行右缘（Flexible 会与 Expanded 平分宽度）。
                  Text(
                    hhText(context, value),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(fontSize: 13, color: p.secondary),
                  ),
                ],
                const SizedBox(width: 4),
                Icon(Icons.chevron_right_rounded, size: 20, color: p.secondary),
              ],
            ),
          ),
        ),
      ),
    );
  }

  void _selectRow(BuildContext context) {
    // Debounce matches the previous list behavior.
    final state = context.findAncestorStateOfType<_SettingsScreenState>();
    if (state == null || !state.mounted || state._opening) return;
    state._select(entry);
  }
}

class _SettingsPage extends StatelessWidget {
  const _SettingsPage({required this.title, required this.child, this.onBack});
  final String title;
  final Widget child;
  final VoidCallback? onBack;
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return Scaffold(
      backgroundColor: p.surface,
      appBar: AppBar(
        backgroundColor: p.surface,
        foregroundColor: p.ink,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        scrolledUnderElevation: 0,
        centerTitle: false,
        leading: BackButton(
          onPressed: onBack ?? () => Navigator.of(context).maybePop(),
        ),
        titleSpacing: 4,
        title: Text(
          hhText(context, title),
          style: TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w700,
            color: p.ink,
          ),
        ),
      ),
      body: SafeArea(
        top: false,
        child: Align(
          alignment: Alignment.topCenter,
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 600),
            child: child,
          ),
        ),
      ),
    );
  }
}

class _SettingsUnavailable extends StatelessWidget {
  const _SettingsUnavailable({required this.title, required this.message});
  final String title, message;
  @override
  Widget build(BuildContext context) => _SettingsPage(
    title: title,
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Text(hhText(context, message)),
    ),
  );
}

/// Shared radio row used by the theme and font-size pages.
class _SettingsChoice extends StatelessWidget {
  const _SettingsChoice({
    required this.title,
    this.subtitle,
    this.leading,
    required this.selected,
    required this.onTap,
  });
  final String title;
  final String? subtitle;
  final Widget? leading;
  final bool selected;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return Semantics(
      selected: selected,
      child: HhPressFeedback(
        pressedScale: .985,
        builder: (context, states) => InkWell(
          statesController: states,
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 15),
            child: Row(
              children: [
                if (leading != null) ...[leading!, const SizedBox(width: 14)],
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        hhText(context, title),
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w500,
                          color: p.ink,
                        ),
                      ),
                      if (subtitle != null) ...[
                        const SizedBox(height: 3),
                        Text(
                          hhText(context, subtitle!),
                          style: TextStyle(
                            fontSize: 12,
                            height: 1.5,
                            color: p.secondary,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
                HhAnimatedSelection(
                  selected: selected,
                  builder: (context, progress) => ExcludeSemantics(
                    child: Stack(
                      children: [
                        Icon(
                          Icons.radio_button_unchecked,
                          color: Color.lerp(p.secondary, p.accent, progress),
                        ),
                        Opacity(
                          opacity: progress,
                          child: Icon(
                            Icons.radio_button_checked,
                            color: p.accent,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _ThemeSettings extends StatefulWidget {
  const _ThemeSettings();
  @override
  State<_ThemeSettings> createState() => _ThemeSettingsState();
}

class _ThemeSettingsState extends State<_ThemeSettings> {
  String? _message;
  int _revision = 0;
  Future<void> _choose(
    AuthPresentationController controller,
    ThemeMode mode,
  ) async {
    final revision = ++_revision;
    setState(() => _message = null);
    controller.setThemeMode(mode);
    try {
      await controller.preferencesSaved;
    } catch (_) {
      if (mounted && revision == _revision) {
        setState(() => _message = '主题已切换，但未能保存到本机，请重试。');
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final controller = AuthPresentationScope.maybeOf(context);
    final p = HhTelegramPalette.of(context);
    final modes = [
      (ThemeMode.light, '浅色', '始终使用明亮的浅色界面'),
      (ThemeMode.dark, '深色', '始终使用低亮度的深色界面'),
      (ThemeMode.system, '跟随系统', '随手机系统的深浅色设置自动切换'),
    ];
    return _SettingsPage(
      title: '主题',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const SizedBox(height: 20),
          DecoratedBox(
            decoration: BoxDecoration(
              color: p.surface,
              border: Border(
                top: BorderSide(color: p.line),
                bottom: BorderSide(color: p.line),
              ),
            ),
            child: ListenableBuilder(
              listenable: controller ?? const ListenableNever(),
              builder: (context, _) => Column(
                children: [
                  for (var i = 0; i < modes.length; i++)
                    DecoratedBox(
                      decoration: BoxDecoration(
                        border: i < modes.length - 1
                            ? Border(bottom: BorderSide(color: p.line))
                            : null,
                      ),
                      child: _SettingsChoice(
                        title: modes[i].$2,
                        subtitle: modes[i].$3,
                        leading: _ThemeSwatch(mode: modes[i].$1),
                        selected:
                            (controller?.themeMode ?? ThemeMode.system) ==
                            modes[i].$1,
                        onTap: controller == null
                            ? null
                            : () => _choose(controller, modes[i].$1),
                      ),
                    ),
                ],
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(22, 14, 22, 0),
            child: Text(
              hhText(context, _message ?? '主题只影响本机显示，不改变账号数据。'),
              style: TextStyle(fontSize: 12, height: 1.8, color: p.secondary),
            ),
          ),
        ],
      ),
    );
  }
}

class _ThemeSwatch extends StatelessWidget {
  const _ThemeSwatch({required this.mode});
  final ThemeMode mode;
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return Container(
      width: 26,
      height: 26,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        gradient: mode == ThemeMode.system
            ? LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [Colors.white, p.ink.withValues(alpha: .85)],
              )
            : null,
        color: mode == ThemeMode.light
            ? Colors.white
            : mode == ThemeMode.dark
            ? p.ink.withValues(alpha: .85)
            : null,
        border: Border.all(color: p.line),
      ),
    );
  }
}

class _FontSizeSettings extends StatefulWidget {
  const _FontSizeSettings();
  @override
  State<_FontSizeSettings> createState() => _FontSizeSettingsState();
}

class _FontSizeSettingsState extends State<_FontSizeSettings> {
  String? _message;
  int _revision = 0;
  Future<void> _choose(
    AuthPresentationController controller,
    String key,
  ) async {
    final revision = ++_revision;
    setState(() => _message = null);
    controller.setFontScale(key);
    try {
      await controller.preferencesSaved;
    } catch (_) {
      if (mounted && revision == _revision) {
        setState(() => _message = '字体大小已切换，但未能保存到本机，请重试。');
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final controller = AuthPresentationScope.maybeOf(context);
    final p = HhTelegramPalette.of(context);
    const sizes = [
      ('small', '小'),
      ('standard', '标准'),
      ('large', '大'),
      ('xlarge', '特大'),
    ];
    return _SettingsPage(
      title: '字体大小',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const SizedBox(height: 20),
          DecoratedBox(
            decoration: BoxDecoration(
              color: p.surface,
              border: Border(
                top: BorderSide(color: p.line),
                bottom: BorderSide(color: p.line),
              ),
            ),
            child: ListenableBuilder(
              listenable: controller ?? const ListenableNever(),
              builder: (context, _) => Column(
                children: [
                  for (var i = 0; i < sizes.length; i++)
                    DecoratedBox(
                      decoration: BoxDecoration(
                        border: i < sizes.length - 1
                            ? Border(bottom: BorderSide(color: p.line))
                            : null,
                      ),
                      child: _SettingsChoice(
                        title: sizes[i].$2,
                        selected:
                            (controller?.fontScale ?? 'standard') ==
                            sizes[i].$1,
                        onTap: controller == null
                            ? null
                            : () => _choose(controller, sizes[i].$1),
                      ),
                    ),
                ],
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(20),
            child: DecoratedBox(
              decoration: BoxDecoration(
                color: p.surface,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: p.line),
              ),
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      hhText(context, '预览标题'),
                      style: TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w600,
                        color: p.ink,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      hhText(
                        context,
                        '这是正文的预览效果。调整字号后，整个应用的界面文字会随之变大或变小，可返回设置与“我的”页查看。',
                      ),
                      style: TextStyle(
                        fontSize: 14,
                        height: 1.75,
                        color: p.ink.withValues(alpha: .88),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 0),
            child: Text(
              hhText(context, _message ?? '字体大小只影响本机显示。'),
              style: TextStyle(fontSize: 12, height: 1.8, color: p.secondary),
            ),
          ),
        ],
      ),
    );
  }
}

class _AccountProfile extends StatefulWidget {
  const _AccountProfile({this.accountId, this.username});
  final String? accountId, username;
  @override
  State<_AccountProfile> createState() => _AccountProfileState();
}

class _AccountProfileState extends State<_AccountProfile> {
  bool _copied = false;
  Timer? _copyTimer;
  @override
  void dispose() {
    _copyTimer?.cancel();
    super.dispose();
  }

  Future<void> _copy(String value) async {
    try {
      await Clipboard.setData(ClipboardData(text: value));
      if (!mounted) return;
      _copyTimer?.cancel();
      setState(() => _copied = true);
      _copyTimer = Timer(const Duration(milliseconds: 1600), () {
        if (mounted) setState(() => _copied = false);
      });
    } catch (_) {
      // Platform clipboard access can be unavailable; the value stays visible.
    }
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final rows = <(String, String?, Widget?)>[
      if (widget.accountId case final id?)
        (
          '平台编号',
          id,
          TextButton(
            key: const ValueKey('settings-account-copy'),
            style: TextButton.styleFrom(
              foregroundColor: p.accent,
              padding: const EdgeInsets.symmetric(horizontal: 12),
              minimumSize: const Size(0, 32),
            ),
            onPressed: () => _copy(id),
            child: Text(hhText(context, _copied ? '已复制' : '复制')),
          ),
        ),
      if (widget.username case final name?) ('用户名', name, null),
    ];
    return _SettingsPage(
      title: '账号资料',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(22, 14, 22, 8),
            child: Text(
              hhText(context, '账号信息属于本人私有资料，与公开发言身份相互独立。'),
              style: TextStyle(fontSize: 12, height: 1.8, color: p.secondary),
            ),
          ),
          if (rows.isEmpty)
            Padding(
              padding: const EdgeInsets.all(24),
              child: Text(
                hhText(context, '暂无可展示的账号资料。'),
                style: TextStyle(color: p.secondary),
              ),
            )
          else ...[
            DecoratedBox(
              decoration: BoxDecoration(
                color: p.surface,
                border: Border(
                  top: BorderSide(color: p.line),
                  bottom: BorderSide(color: p.line),
                ),
              ),
              child: Column(
                children: [
                  for (var i = 0; i < rows.length; i++)
                    DecoratedBox(
                      decoration: BoxDecoration(
                        border: i < rows.length - 1
                            ? Border(bottom: BorderSide(color: p.line))
                            : null,
                      ),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 20,
                          vertical: 14,
                        ),
                        child: Row(
                          children: [
                            // 标题保持固有宽度；长编号只在值的剩余空间内省略，
                            // 不再挤压标题或把复制按钮推出屏幕。
                            Text(
                              hhText(context, rows[i].$1),
                              maxLines: 1,
                              style: TextStyle(fontSize: 14, color: p.ink),
                            ),
                            const SizedBox(width: 16),
                            Expanded(
                              child: Text(
                                rows[i].$2!,
                                maxLines: 1,
                                textAlign: TextAlign.right,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(
                                  fontSize: 14,
                                  color: p.secondary,
                                ),
                              ),
                            ),
                            if (rows[i].$3 != null) rows[i].$3!,
                          ],
                        ),
                      ),
                    ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(22, 14, 22, 0),
              child: Text(
                hhText(context, '平台编号仅用于本人核对与意见反馈，不会在公开内容或私信中展示。'),
                style: TextStyle(fontSize: 12, height: 1.8, color: p.secondary),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _AboutSettings extends StatelessWidget {
  const _AboutSettings();
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return _SettingsPage(
      title: '关于 SpeechFUL',
      child: ListView(
        key: const ValueKey('settings-about-page'),
        padding: const EdgeInsets.all(20),
        children: [
          DecoratedBox(
            decoration: BoxDecoration(
              color: p.surface,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: p.line),
            ),
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'SpeechFUL',
                    style: TextStyle(
                      fontSize: 17,
                      fontWeight: FontWeight.w600,
                      color: p.ink,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    hhText(context, '版本v2.0.0'),
                    key: const ValueKey('settings-about-version'),
                    style: TextStyle(
                      fontSize: 14,
                      color: p.ink.withValues(alpha: .88),
                    ),
                  ),
                  const SizedBox(height: 16),
                  Text(
                    hhText(context, '面向海南大学校内人员的匿名社区，让世界多一些爱与真诚。'),
                    key: const ValueKey('settings-about-intro'),
                    style: TextStyle(
                      fontSize: 14,
                      height: 1.75,
                      color: p.ink.withValues(alpha: .88),
                    ),
                  ),
                  const SizedBox(height: 14),
                  Text(
                    hhText(
                      context,
                      '在这里，你可以按校园分类浏览、搜索和分享帖子，发布文字或图文内容，并通过回复、点赞、通知和私信参与交流。',
                    ),
                    key: const ValueKey('settings-about-features'),
                    style: TextStyle(
                      fontSize: 14,
                      height: 1.75,
                      color: p.secondary,
                    ),
                  ),
                  const SizedBox(height: 14),
                  Text(
                    hhText(
                      context,
                      '每个账号可以拥有多个长期发言身份。公开内容只展示你选择的身份，账号资料及不同身份之间的关联不会公开展示。',
                    ),
                    key: const ValueKey('settings-about-identities'),
                    style: TextStyle(
                      fontSize: 14,
                      height: 1.75,
                      color: p.secondary,
                    ),
                  ),
                  const SizedBox(height: 14),
                  Text(
                    hhText(
                      context,
                      'SpeechFUL 为独立开发的校园社区项目，与海南大学不存在官方隶属、认可或授权关系。',
                    ),
                    key: const ValueKey('settings-about-disclaimer'),
                    style: TextStyle(
                      fontSize: 14,
                      height: 1.75,
                      color: p.secondary,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _LanguageSettings extends StatefulWidget {
  const _LanguageSettings();
  @override
  State<_LanguageSettings> createState() => _LanguageSettingsState();
}

class _LanguageSettingsState extends State<_LanguageSettings> {
  String? _message;
  int _revision = 0;
  Future<void> _choose(
    AuthPresentationController controller,
    AppLanguage language,
  ) async {
    final revision = ++_revision;
    setState(() => _message = null);
    controller.setLanguage(language);
    try {
      await controller.preferencesSaved;
    } catch (_) {
      if (mounted && revision == _revision) {
        setState(() => _message = '语言已切换，但未能保存到本机，请重试。');
      }
    }
  }

  @override
  Widget build(BuildContext context) => AuthPresentationHost(
    child: Builder(
      builder: (context) {
        final controller = AuthPresentationScope.maybeOf(context)!;
        final p = HhTelegramPalette.of(context);
        return _SettingsPage(
          title: '语言',
          child: ListView(
            key: const ValueKey('settings-language-page'),
            padding: const EdgeInsets.all(20),
            children: [
              for (final language in AppLanguage.values)
                Semantics(
                  selected: controller.language == language,
                  child: HhPressFeedback(
                    pressedScale: .985,
                    builder: (context, states) => ListTile(
                      statesController: states,
                      key: ValueKey('settings-language-${language.code}'),
                      contentPadding: EdgeInsets.zero,
                      title: Text(
                        language.nativeName,
                        style: const TextStyle(fontSize: 17),
                      ),
                      trailing: HhAnimatedSelection(
                        selected: controller.language == language,
                        builder: (context, progress) => ExcludeSemantics(
                          child: Stack(
                            children: [
                              Icon(
                                Icons.radio_button_unchecked,
                                color: Color.lerp(
                                  p.secondary,
                                  p.accent,
                                  progress,
                                ),
                              ),
                              Opacity(
                                opacity: progress,
                                child: Icon(
                                  Icons.radio_button_checked,
                                  color: p.accent,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                      onTap: () => _choose(controller, language),
                    ),
                  ),
                ),
              if (_message != null)
                Semantics(
                  liveRegion: true,
                  child: Text(hhText(context, _message!)),
                ),
            ],
          ),
        );
      },
    ),
  );
}

class _FeedbackSettings extends StatelessWidget {
  const _FeedbackSettings();
  static const email = 'zewbby666@gmail.com';
  @override
  Widget build(BuildContext context) => _SettingsPage(
    title: '意见反馈',
    child: ListView(
      key: const ValueKey('settings-feedback-page'),
      padding: const EdgeInsets.all(24),
      children: [
        Text(
          hhText(context, '联系方式'),
          style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 24),
        Text(
          hhText(context, '邮箱'),
          style: TextStyle(color: HhTelegramPalette.of(context).secondary),
        ),
        const SizedBox(height: 8),
        const SelectableText(
          email,
          key: ValueKey('settings-feedback-email'),
          style: TextStyle(fontSize: 18),
        ),
        const SizedBox(height: 20),
        HhPressFeedback(
          builder: (context, states) => OutlinedButton.icon(
            statesController: states,
            key: const ValueKey('settings-feedback-copy'),
            icon: const Icon(Icons.copy_outlined),
            label: Text(hhText(context, '复制邮箱')),
            onPressed: () async {
              String message;
              try {
                await Clipboard.setData(const ClipboardData(text: email));
                message = '邮箱已复制';
              } catch (_) {
                message = '复制失败，请重试';
              }
              if (context.mounted) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text(hhText(context, message))),
                );
              }
            },
          ),
        ),
      ],
    ),
  );
}

class _SupportSettings extends StatefulWidget {
  const _SupportSettings({this.gallery});
  final ViewerGalleryWriter? gallery;
  @override
  State<_SupportSettings> createState() => _SupportSettingsState();
}

class _SupportSettingsState extends State<_SupportSettings>
    with WidgetsBindingObserver {
  static const asset = 'assets/support/alipay-receive.jpg';
  late final _saver = ViewerSaveCoordinator(
    gallery: widget.gallery ?? const GalViewerGalleryWriter(),
  );
  bool _busy = false, _foreground = true;
  String? _message;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _foreground =
        state != AppLifecycleState.paused &&
        state != AppLifecycleState.detached;
  }

  Future<void> _save() async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _message = null;
    });
    final result = await _saver.save(
      itemId: 'speechful-alipay-receive-v1',
      resolveOriginal: () async => ViewerOriginal.bytes(
        (await rootBundle.load(asset)).buffer.asUint8List(),
        ViewerOriginalFormat.jpeg,
      ),
      isCurrent: () =>
          mounted && _foreground && ModalRoute.isCurrentOf(context) != false,
    );
    if (!mounted) return;
    setState(() {
      _busy = false;
      if (!result.isCurrent) return;
      _message = switch (result.status) {
        ViewerSaveStatus.saved => '收款码已保存到相册',
        ViewerSaveStatus.permissionDenied => '请允许访问相册后重试',
        ViewerSaveStatus.outcomeUnknown => '保存结果尚未确定，请先查看相册',
        _ => '暂时无法保存，请重试',
      };
    });
  }

  Future<void> _showCode() => showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    isScrollControlled: true,
    useSafeArea: true,
    backgroundColor: HhTelegramPalette.of(context).surface,
    builder: (sheet) {
      final p = HhTelegramPalette.of(sheet);
      final maxCodeHeight = MediaQuery.sizeOf(sheet).height * .52;
      return SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              hhText(sheet, '支付宝收款码'),
              style: TextStyle(
                fontSize: 17,
                fontWeight: FontWeight.w600,
                color: p.ink,
              ),
            ),
            const SizedBox(height: 14),
            ConstrainedBox(
              constraints: BoxConstraints(
                maxWidth: 300,
                maxHeight: maxCodeHeight,
              ),
              child: AspectRatio(
                aspectRatio: 2 / 3,
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(12),
                  child: Image.asset(
                    asset,
                    key: const ValueKey('settings-support-code'),
                    fit: BoxFit.contain,
                    semanticLabel: hhText(sheet, '支付宝收款码'),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 14),
            Text(
              hhText(sheet, '使用支付宝扫一扫，或保存收款码后从相册识别。'),
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 13, height: 1.6, color: p.secondary),
            ),
          ],
        ),
      );
    },
  );
  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _saver.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return _SettingsPage(
      title: '支持我们',
      child: ColoredBox(
        color: p.background,
        child: ListView(
          key: const ValueKey('settings-support-page'),
          padding: const EdgeInsets.fromLTRB(20, 18, 20, 28),
          children: [
            Row(
              key: const ValueKey('settings-support-intro'),
              children: [
                Container(
                  width: 44,
                  height: 44,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: p.accent.withValues(alpha: .12),
                  ),
                  child: ExcludeSemantics(
                    child: Icon(
                      Icons.favorite_rounded,
                      size: 22,
                      color: p.accent,
                    ),
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Text(
                    hhText(context, '感谢你支持 SpeechFUL'),
                    style: TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w600,
                      height: 1.4,
                      color: p.ink,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 18),
            _SupportCard(
              key: const ValueKey('settings-support-code-card'),
              padding: EdgeInsets.zero,
              child: HhPressFeedback(
                pressedScale: .99,
                builder: (context, states) => Material(
                  color: Colors.transparent,
                  borderRadius: BorderRadius.circular(16),
                  clipBehavior: Clip.antiAlias,
                  child: InkWell(
                    key: const ValueKey('settings-support-code-entry'),
                    statesController: states,
                    onTap: _showCode,
                    child: Padding(
                      padding: const EdgeInsets.fromLTRB(16, 14, 12, 14),
                      child: Row(
                        children: [
                          Container(
                            width: 40,
                            height: 40,
                            decoration: BoxDecoration(
                              shape: BoxShape.circle,
                              color: p.accent.withValues(alpha: .12),
                            ),
                            child: ExcludeSemantics(
                              child: Icon(
                                Icons.qr_code_2_rounded,
                                size: 21,
                                color: p.accent,
                              ),
                            ),
                          ),
                          const SizedBox(width: 12),
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  hhText(context, '支付宝收款码'),
                                  style: TextStyle(
                                    fontSize: 15,
                                    fontWeight: FontWeight.w600,
                                    height: 1.35,
                                    color: p.ink,
                                  ),
                                ),
                                const SizedBox(height: 2),
                                Text(
                                  hhText(context, '点击查看收款码'),
                                  style: TextStyle(
                                    fontSize: 12,
                                    height: 1.4,
                                    color: p.secondary,
                                  ),
                                ),
                              ],
                            ),
                          ),
                          ExcludeSemantics(
                            child: Icon(
                              Icons.chevron_right_rounded,
                              size: 22,
                              color: p.secondary,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 14),
            _SupportCard(
              key: const ValueKey('settings-support-purpose-card'),
              padding: const EdgeInsets.fromLTRB(16, 14, 16, 14),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                    width: 34,
                    height: 34,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: p.accent.withValues(alpha: .12),
                    ),
                    child: ExcludeSemantics(
                      child: Icon(
                        Icons.volunteer_activism_outlined,
                        size: 18,
                        color: p.accent,
                      ),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      hhText(context, '您的支持将全数用于开发与维护SpeechFUL项目'),
                      key: const ValueKey('settings-support-purpose'),
                      style: TextStyle(
                        fontSize: 14,
                        height: 1.65,
                        color: p.ink.withValues(alpha: .88),
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 14),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 4),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  ExcludeSemantics(
                    child: Icon(
                      Icons.qr_code_scanner_rounded,
                      size: 18,
                      color: p.secondary,
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      hhText(context, '使用支付宝扫一扫，或保存收款码后从相册识别。'),
                      key: const ValueKey('settings-support-instructions'),
                      style: TextStyle(
                        fontSize: 13,
                        height: 1.6,
                        color: p.secondary,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 20),
            HhPressFeedback(
              enabled: !_busy,
              builder: (context, states) => FilledButton.icon(
                statesController: states,
                key: const ValueKey('settings-support-save'),
                onPressed: _busy ? null : _save,
                style: FilledButton.styleFrom(
                  minimumSize: const Size.fromHeight(48),
                ),
                icon: const Icon(Icons.save_alt_rounded, size: 20),
                label: Text(hhText(context, _busy ? '保存中…' : '保存收款码')),
              ),
            ),
            if (_message != null)
              Padding(
                padding: const EdgeInsets.only(top: 14),
                child: Semantics(
                  key: const ValueKey('settings-support-status'),
                  liveRegion: true,
                  child: Text(
                    hhText(context, _message!),
                    textAlign: TextAlign.center,
                    style: TextStyle(fontSize: 14, height: 1.5, color: p.ink),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

/// Rounded surface used by the support page's card sections.
class _SupportCard extends StatelessWidget {
  const _SupportCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(16),
  });
  final Widget child;
  final EdgeInsetsGeometry padding;
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return DecoratedBox(
      decoration: BoxDecoration(
        color: p.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: p.line),
      ),
      child: Padding(padding: padding, child: child),
    );
  }
}
