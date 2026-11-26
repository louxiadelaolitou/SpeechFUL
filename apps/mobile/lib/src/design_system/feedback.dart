import 'package:flutter/material.dart';

import '../localization/app_localizations.dart';
import 'telegram_theme.dart';

/// 应用内轻量反馈的唯一视觉实现。
///
/// 所有同类提示（「不感兴趣」归属提示、分享失败、拉黑结果、头像失败、
/// 评论图片失败、私信操作失败等）都必须经过这里，避免旧样式残留。
///
/// SnackBar 只作为计时与展示宿主（透明、零内边距），可见的胶囊由
/// [_HhFeedbackContent] 自绘：配色取自 [HhTelegramPalette]，高度随文案紧凑
/// 变化；左右滑动时整颗胶囊（容器、图标、文字）一起跟手位移。
///
/// 交互：显示后 2 秒自动关闭；右侧叉号可立即关闭；左右滑动可移除。
/// 新提示到来时先关闭上一条，不排队叠加；关闭按钮不取消自动关闭
/// （显式 `persist: false`），切页时仍跟随根 ScaffoldMessenger。
void showHhFeedback(
  BuildContext context,
  String message, {
  bool replaceCurrent = true,
}) {
  final messenger = ScaffoldMessenger.maybeOf(context);
  if (messenger == null) return;
  if (replaceCurrent) messenger.hideCurrentSnackBar();
  messenger.showSnackBar(
    SnackBar(
      key: const ValueKey('hh-feedback'),
      duration: const Duration(seconds: 2),
      behavior: SnackBarBehavior.floating,
      // 水平拖动完全由内容区接管，避免 SDK 默认单向滑动手势竞争。
      dismissDirection: DismissDirection.none,
      // 带关闭按钮时 SDK 默认 persist=true，定时器不会生效，必须显式关掉。
      persist: false,
      // 宿主只负责计时：去掉 SDK 的底色、阴影与内边距，胶囊由内容自绘。
      backgroundColor: Colors.transparent,
      elevation: 0,
      padding: EdgeInsets.zero,
      content: _HhFeedbackContent(message: hhText(context, message)),
    ),
  );
}

class _HhFeedbackContent extends StatefulWidget {
  const _HhFeedbackContent({required this.message});
  final String message;

  @override
  State<_HhFeedbackContent> createState() => _HhFeedbackContentState();
}

class _HhFeedbackContentState extends State<_HhFeedbackContent> {
  double _dragX = 0;

  // 拖拽开始时记录胶囊宽度：build 阶段不能读 context.size，先缓存再算透明度
  // 与关闭阈值。
  double _width = 0;

  void _close() {
    // 同一个根 messenger 让跨页时也能关闭当前提示，而不是只关当前子路由。
    final messenger = ScaffoldMessenger.maybeOf(context);
    if (messenger == null) return;
    messenger.hideCurrentSnackBar();
    if (mounted) setState(() => _dragX = 0);
  }

  double get _fade =>
      _width <= 0 ? 1 : (1 - _dragX.abs() / (_width * 1.6)).clamp(.55, 1);

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return GestureDetector(
      key: const ValueKey('hh-feedback-swipe'),
      behavior: HitTestBehavior.opaque,
      onHorizontalDragStart: (_) => _width = context.size?.width ?? 0,
      onHorizontalDragUpdate: (details) {
        setState(() => _dragX += details.delta.dx);
      },
      onHorizontalDragEnd: (details) {
        final width = _width > 0 ? _width : 320.0;
        final fast = (details.primaryVelocity ?? 0).abs() > 400;
        if (fast || _dragX.abs() > width * .3) {
          _close();
        } else {
          setState(() => _dragX = 0);
        }
      },
      onHorizontalDragCancel: () => setState(() => _dragX = 0),
      // 位移作用在整颗胶囊上，容器、图标与文字一起移动。
      child: Transform.translate(
        offset: Offset(_dragX, 0),
        child: Opacity(
          opacity: _fade,
          child: Material(
            key: const ValueKey('hh-feedback-card'),
            color: p.surface,
            elevation: 6,
            shadowColor: Colors.black.withValues(alpha: .18),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
              side: BorderSide(color: p.line),
            ),
            clipBehavior: Clip.antiAlias,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(14, 2, 2, 2),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      widget.message,
                      maxLines: 3,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 13.5,
                        height: 1.35,
                        color: p.ink,
                      ),
                    ),
                  ),
                  SizedBox(
                    width: 34,
                    height: 34,
                    child: IconButton(
                      key: const ValueKey('hh-feedback-close'),
                      tooltip: hhText(context, '关闭'),
                      padding: EdgeInsets.zero,
                      constraints: const BoxConstraints.tightFor(
                        width: 34,
                        height: 34,
                      ),
                      visualDensity: VisualDensity.compact,
                      iconSize: 18,
                      icon: Icon(Icons.close_rounded, color: p.secondary),
                      onPressed: _close,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
