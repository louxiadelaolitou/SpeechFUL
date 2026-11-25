import 'package:flutter/material.dart';

import '../localization/app_localizations.dart';

import '../design_system/motion.dart';
import '../design_system/motion_routes.dart';
import '../design_system/telegram_theme.dart';
import '../messaging/direct_message_models.dart';
import '../messaging/direct_message_composer.dart';
import 'identity_api.dart';

class IdentitySwitchItem {
  const IdentitySwitchItem(this.identity, {this.unread = 0});
  final DmIdentity identity;
  final int unread;
}

/// 身份胶囊：通知页、私信页与「我的」页共用同一实现，
/// 保证几何尺寸、按压反馈、箭头旋转与昵称/头像切换动画一致。
class HhIdentitySwitcher extends StatelessWidget {
  const HhIdentitySwitcher({
    super.key,
    required this.identity,
    required this.anchor,
    required this.onTap,
    this.menuOpen = false,
    this.otherUnread = false,
    this.enabled = true,
    this.controlKey = const ValueKey('dm-inbox-identity'),
  });
  final DmIdentity identity;
  final GlobalKey anchor;
  final VoidCallback onTap;
  final bool menuOpen, otherUnread, enabled;
  final Key controlKey;

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final duration = HhMotion.duration(context, HhMotion.quick);
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 224, minHeight: 48),
      child: Tooltip(
        message: hhText(context, '切换身份'),
        child: Semantics(
          label: hhText(context, '切换身份，当前{v0}', args: {'v0': identity.name}),
          button: true,
          excludeSemantics: true,
          onTap: enabled ? onTap : null,
          child: HhPressFeedback(
            enabled: enabled,
            builder: (context, states) => InkWell(
              key: controlKey,
              statesController: states,
              onTap: enabled ? onTap : null,
              borderRadius: BorderRadius.circular(24),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 6),
                child: Material(
                  key: anchor,
                  color: p.surface,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(20),
                    side: BorderSide(color: p.line),
                  ),
                  clipBehavior: Clip.antiAlias,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 10,
                      vertical: 4,
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        AnimatedRotation(
                          turns: menuOpen ? 0.25 : 0,
                          duration: duration,
                          curve: HhMotion.curve,
                          child: Icon(
                            Icons.chevron_right_rounded,
                            size: 20,
                            color: p.secondary,
                          ),
                        ),
                        const SizedBox(width: 4),
                        Flexible(
                          child: AnimatedSwitcher(
                            duration: duration,
                            switchInCurve: HhMotion.curve,
                            switchOutCurve: HhMotion.curve,
                            layoutBuilder: (current, previous) => Stack(
                              alignment: Alignment.centerRight,
                              children: [
                                ...previous,
                                if (current != null) current,
                              ],
                            ),
                            transitionBuilder: (child, animation) =>
                                FadeTransition(
                                  opacity: animation,
                                  child: SlideTransition(
                                    position:
                                        Tween<Offset>(
                                          begin: const Offset(.12, 0),
                                          end: Offset.zero,
                                        ).animate(animation),
                                    child: child,
                                  ),
                                ),
                            child: Text(
                              identity.name,
                              key: ValueKey(identity.id),
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 14,
                                fontWeight: FontWeight.w500,
                                color: p.ink,
                              ),
                            ),
                          ),
                        ),
                        const SizedBox(width: 8),
                        AnimatedSwitcher(
                          duration: duration,
                          switchInCurve: HhMotion.curve,
                          switchOutCurve: HhMotion.curve,
                          layoutBuilder: (current, previous) => Stack(
                            alignment: Alignment.center,
                            children: [
                              ...previous,
                              if (current != null) current,
                            ],
                          ),
                          transitionBuilder: (child, animation) =>
                              FadeTransition(opacity: animation, child: child),
                          child: Badge(
                            key: ValueKey(identity.id),
                            isLabelVisible: otherUnread,
                            child: HhDmAvatar(identity: identity, size: 28),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

Future<String?> showHhIdentityMenu({
  required BuildContext context,
  required GlobalKey anchorKey,
  required List<IdentitySwitchItem> identities,
  required String selectedId,
  bool canAdd = false,
  ValueChanged<Route<dynamic>>? onRoute,
  String keyPrefix = 'inbox',
}) {
  final anchor = anchorKey.currentContext!.findRenderObject()! as RenderBox;
  final overlay =
      Navigator.of(context).overlay!.context.findRenderObject()! as RenderBox;
  final rect =
      anchor.localToGlobal(Offset.zero, ancestor: overlay) & anchor.size;
  final p = HhTelegramPalette.of(context);
  return showMenu<String>(
    context: context,
    positionBuilder: (context, constraints) {
      final current = anchor.attached && overlay.attached
          ? anchor.localToGlobal(Offset.zero, ancestor: overlay) & anchor.size
          : rect;
      return RelativeRect.fromRect(
        Rect.fromLTWH(current.right, current.bottom + 6, 0, 0),
        Offset.zero & constraints.biggest,
      );
    },
    constraints: const BoxConstraints(minWidth: 208, maxWidth: 260),
    color: p.surface,
    surfaceTintColor: Colors.transparent,
    elevation: 6,
    shape: RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(16),
      side: BorderSide(color: p.line),
    ),
    popUpAnimationStyle: HhMotionRoutes.popup(context),
    items: [
      PopupMenuItem<String>(
        enabled: false,
        height: 40,
        child: Builder(
          builder: (context) {
            final route = ModalRoute.of(context);
            if (route != null) onRoute?.call(route);
            return Text(
              hhText(context, '切换身份'),
              style: TextStyle(fontSize: 13, color: p.secondary),
            );
          },
        ),
      ),
      for (final item in identities)
        PopupMenuItem<String>(
          key: ValueKey('$keyPrefix-identity:${item.identity.id}'),
          value: item.identity.id,
          height: 56,
          child: Row(
            children: [
              Badge(
                isLabelVisible: item.unread > 0,
                child: HhDmAvatar(identity: item.identity, size: 30),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  item.identity.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(fontSize: 15, color: p.ink),
                ),
              ),
              if (item.identity.id == selectedId)
                Icon(Icons.check_rounded, color: p.accent, size: 20),
            ],
          ),
        ),
      if (identities.length < IdentityDirectory.maximumIdentities)
        PopupMenuItem<String>(
          key: ValueKey('$keyPrefix-add-identity'),
          value: '@add',
          enabled: canAdd,
          height: 56,
          child: Semantics(
            label: hhText(context, '添加身份'),
            child: Center(
              child: Icon(Icons.add_rounded, color: p.accent, size: 26),
            ),
          ),
        ),
    ],
  );
}
