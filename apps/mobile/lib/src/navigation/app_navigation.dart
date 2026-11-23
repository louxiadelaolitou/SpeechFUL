import 'dart:math' as math;

import '../localization/app_localizations.dart';

import 'package:flutter/material.dart';

import '../design_system/telegram_theme.dart';
import '../design_system/motion.dart';
import 'feed_chrome.dart';

enum AppDestination { home, messages, directMessages, personal }

typedef HhNavigationUnread = ({bool messages, bool directMessages});

const HhNavigationUnread hhNoNavigationUnread = (
  messages: false,
  directMessages: false,
);

/// Shared fixed root navigation. Notification and private-message destinations
/// are independent; unread/receipt state is never changed by this widget.
class HhAppNavigation extends StatelessWidget {
  const HhAppNavigation({
    super.key,
    required this.selected,
    required this.onSelected,
    this.composeSlot = false,
    this.onCompose,
    this.composeTooltip = '写帖子',
    this.messagesUnread = false,
    this.directMessagesUnread = false,
  });
  final AppDestination selected;
  final ValueChanged<AppDestination> onSelected;
  final bool composeSlot;
  final VoidCallback? onCompose;
  final String composeTooltip;
  final bool messagesUnread, directMessagesUnread;

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return DecoratedBox(
      decoration: BoxDecoration(
        color: p.surface,
        border: Border(top: BorderSide(color: p.line, width: .5)),
      ),
      child: SafeArea(
        top: false,
        child: SizedBox(
          height: 64,
          child: Row(
            children: [
              for (final destination in AppDestination.values) ...[
                if (composeSlot && destination == AppDestination.directMessages)
                  Expanded(
                    child: Center(
                      child: onCompose == null
                          ? const SizedBox(width: 52, height: 48)
                          : FeedComposeButton(
                              onPressed: onCompose!,
                              tooltip: composeTooltip,
                            ),
                    ),
                  ),
                Expanded(
                  child: _NavButton(
                    destination: destination,
                    selected: selected == destination,
                    unread: switch (destination) {
                      AppDestination.messages => messagesUnread,
                      AppDestination.directMessages => directMessagesUnread,
                      _ => false,
                    },
                    onPressed: () {
                      if (selected != destination) onSelected(destination);
                    },
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

/// 底部导航按钮：选中色／填充过渡之外，点击时做一次与帖子卡片一致的
/// 160ms 图标脉冲（减少动效偏好下只变状态）。
class _NavButton extends StatefulWidget {
  const _NavButton({
    required this.destination,
    required this.selected,
    required this.unread,
    required this.onPressed,
  });
  final AppDestination destination;
  final bool selected;
  final bool unread;
  final VoidCallback onPressed;
  @override
  State<_NavButton> createState() => _NavButtonState();
}

class _NavButtonState extends State<_NavButton>
    with SingleTickerProviderStateMixin {
  /// 0→1 的点击动画进度；t=1 时所有位移/旋转归零，保持静止外观。
  late final _tap = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 320),
    value: 1,
  );

  @override
  void dispose() {
    _tap.dispose();
    super.dispose();
  }

  void _handleTap() {
    if (!HhMotion.reduced(context)) _tap.forward(from: 0);
    widget.onPressed();
  }

  /// 每个导航项的动作不同：首页上跳、铃铛左右摇、气泡弹一下、人像轻晃。
  ({double dy, double rotation, double scale}) _motion(double t) {
    final swing = math.sin(t * math.pi);
    final decayed = (1 - t) * math.sin(t * math.pi * 3);
    return switch (widget.destination) {
      AppDestination.home => (
        dy: -4 * swing,
        rotation: 0.0,
        scale: 1 + .10 * swing,
      ),
      AppDestination.messages => (
        dy: 0.0,
        rotation: .28 * decayed,
        scale: 1 + .04 * swing,
      ),
      AppDestination.directMessages => (
        dy: -2.5 * swing,
        rotation: 0.0,
        scale: 1 + .18 * swing,
      ),
      AppDestination.personal => (
        dy: 0.0,
        rotation: .10 * (1 - t) * math.sin(t * math.pi * 2),
        scale: 1 + .08 * swing,
      ),
    };
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final destination = widget.destination;
    final selected = widget.selected;
    final label = switch (destination) {
      AppDestination.home => hhText(context, '首页'),
      AppDestination.messages => hhText(context, '消息'),
      AppDestination.directMessages => hhText(context, '私信'),
      AppDestination.personal => hhText(context, '我的'),
    };
    return Semantics(
      selected: selected,
      child: HhPressFeedback(
        builder: (context, states) => IconButton(
          statesController: states,
          key: ValueKey('destination:${destination.name}'),
          tooltip: widget.unread
              ? hhText(context, '{v0}，有未读', args: {'v0': label})
              : label,
          onPressed: _handleTap,
          icon: Stack(
            clipBehavior: Clip.none,
            children: [
              AnimatedBuilder(
                animation: _tap,
                builder: (context, child) {
                  final motion = _motion(_tap.value);
                  return Transform.translate(
                    offset: Offset(0, motion.dy),
                    child: Transform.rotate(
                      angle: motion.rotation,
                      alignment: Alignment.center,
                      child: Transform.scale(scale: motion.scale, child: child),
                    ),
                  );
                },
                child: HhAnimatedSelection(
                  selected: selected,
                  builder: (context, progress) => Transform.scale(
                    scale: 1 + .06 * progress,
                    child: CustomPaint(
                      size: const Size(27, 27),
                      painter: HhNavigationGlyph(
                        destination,
                        Color.lerp(p.ink, p.action, progress)!,
                        selected,
                        fillProgress: progress,
                      ),
                    ),
                  ),
                ),
              ),
              if (widget.unread)
                Positioned(
                  right: -1,
                  top: -1,
                  child: ExcludeSemantics(
                    child: Container(
                      key: ValueKey('navigation-unread:${destination.name}'),
                      width: 8,
                      height: 8,
                      decoration: BoxDecoration(
                        color: p.action,
                        shape: BoxShape.circle,
                        border: Border.all(color: p.surface, width: 1.5),
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Code-native vectors matching the supplied bell and single speech bubble.
class HhNavigationGlyph extends CustomPainter {
  const HhNavigationGlyph(
    this.destination,
    this.color,
    this.selected, {
    this.fillProgress,
  });
  final AppDestination destination;
  final Color color;
  final bool selected;
  final double? fillProgress;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.width / 28, size.height / 28);
    final pen = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.8
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;
    final fill = fillProgress ?? (selected ? 1.0 : 0.0);
    void shape(Path path) {
      if (fill > 0) {
        canvas.drawPath(
          path,
          Paint()..color = color.withValues(alpha: color.a * fill),
        );
      }
      canvas.drawPath(path, pen);
    }

    switch (destination) {
      case AppDestination.home:
        shape(
          Path()
            ..moveTo(3, 12)
            ..lineTo(14, 3)
            ..lineTo(25, 12)
            ..lineTo(25, 24)
            ..quadraticBezierTo(25, 25, 24, 25)
            ..lineTo(17, 25)
            ..lineTo(17, 17)
            ..lineTo(11, 17)
            ..lineTo(11, 25)
            ..lineTo(4, 25)
            ..quadraticBezierTo(3, 25, 3, 24)
            ..close(),
        );
      case AppDestination.messages:
        shape(
          Path()
            ..moveTo(4, 21)
            ..cubicTo(5.7, 17.6, 5.5, 13.1, 6.4, 9.3)
            ..cubicTo(7.2, 5.7, 9.9, 3.4, 14, 3.4)
            ..cubicTo(18.1, 3.4, 20.8, 5.7, 21.6, 9.3)
            ..cubicTo(22.5, 13.1, 22.3, 17.6, 24, 21)
            ..close(),
        );
        canvas.drawPath(
          Path()
            ..moveTo(10.4, 24)
            ..quadraticBezierTo(14, 28, 17.6, 24),
          pen,
        );
      case AppDestination.directMessages:
        shape(
          Path()
            ..moveTo(14, 3)
            ..cubicTo(7.4, 3, 3, 7.1, 3, 12.7)
            ..cubicTo(3, 16, 4.3, 18.7, 6.7, 20.5)
            ..lineTo(4.6, 25)
            ..lineTo(11.2, 22.4)
            ..cubicTo(18.9, 24.1, 25, 19.5, 25, 12.7)
            ..cubicTo(25, 7.1, 20.5, 3, 14, 3)
            ..close(),
        );
      case AppDestination.personal:
        if (fill > 0) {
          canvas.drawCircle(
            const Offset(14, 8.5),
            4.5,
            Paint()..color = color.withValues(alpha: color.a * fill),
          );
        }
        canvas.drawCircle(const Offset(14, 8.5), 4.5, pen);
        shape(
          Path()
            ..moveTo(4, 25)
            ..lineTo(4, 23)
            ..cubicTo(4, 18.5, 8.2, 16.7, 14, 16.7)
            ..cubicTo(19.8, 16.7, 24, 18.5, 24, 23)
            ..lineTo(24, 25)
            ..close(),
        );
    }
    canvas.restore();
  }

  @override
  bool shouldRepaint(covariant HhNavigationGlyph oldDelegate) =>
      destination != oldDelegate.destination ||
      color != oldDelegate.color ||
      fillProgress != oldDelegate.fillProgress ||
      selected != oldDelegate.selected;
}
