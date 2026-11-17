import 'dart:async';

import 'package:flutter/material.dart';

import '../localization/app_localizations.dart';
import 'captcha.dart';
import 'auth_presentation.dart';

Future<String?> showCaptchaDialog(
  BuildContext context,
  CaptchaExchange exchange,
) {
  FocusManager.instance.primaryFocus?.unfocus();
  final themes = InheritedTheme.capture(
    from: context,
    to: Navigator.of(context, rootNavigator: true).context,
  );
  final presentation = AuthPresentationScope.maybeOf(context);
  return showGeneralDialog<String>(
    context: context,
    barrierDismissible: true,
    barrierLabel: MaterialLocalizations.of(context).modalBarrierDismissLabel,
    transitionDuration: MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : const Duration(milliseconds: 180),
    transitionBuilder: (_, animation, secondaryAnimation, child) =>
        FadeTransition(
          opacity: animation,
          child: ScaleTransition(
            scale: animation
                .drive(CurveTween(curve: Curves.easeOutCubic))
                .drive(Tween<double>(begin: .88, end: 1)),
            child: child,
          ),
        ),
    pageBuilder: (_, animation, secondaryAnimation) => themes.wrap(
      presentation == null
          ? CaptchaDialog(exchange: exchange)
          : AuthPresentationScope(
              controller: presentation,
              child: CaptchaDialog(exchange: exchange),
            ),
    ),
  );
}

class CaptchaDialog extends StatefulWidget {
  const CaptchaDialog({super.key, required this.exchange});
  final CaptchaExchange exchange;
  @override
  State<CaptchaDialog> createState() => _CaptchaDialogState();
}

class _CaptchaDialogState extends State<CaptchaDialog> {
  CaptchaChallenge? _challenge;
  double _x = 0;
  bool _loading = false, _checking = false;
  String? _message;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  Future<void> _load({String? message}) async {
    final generation = ++_generation;
    setState(() {
      _loading = true;
      _challenge = null;
      _message = message;
    });
    try {
      final challenge = await widget.exchange.load();
      if (!mounted || generation != _generation) return;
      setState(() {
        _challenge = challenge;
        _x = challenge.startX.toDouble();
      });
    } on Object {
      if (mounted && generation == _generation) {
        setState(() => _message = '验证加载失败，请重试');
      }
    } finally {
      if (mounted && generation == _generation) {
        setState(() => _loading = false);
      }
    }
  }

  Future<void> _solve(double x) async {
    final challenge = _challenge;
    if (challenge == null || _checking || _loading) return;
    setState(() => _checking = true);
    try {
      final token = await widget.exchange.solve(challenge.id, x);
      if (mounted) Navigator.of(context).pop(token);
    } on Object {
      if (mounted) await _load(message: '验证未通过，请重试');
    } finally {
      if (mounted) setState(() => _checking = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final challenge = _challenge;
    final colors = Theme.of(context).colorScheme;
    return AlertDialog(
      key: const ValueKey('captcha-dialog'),
      insetPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(24)),
      backgroundColor: colors.surface,
      surfaceTintColor: Colors.transparent,
      titleTextStyle: Theme.of(context).textTheme.titleLarge
          ?.copyWith(fontWeight: FontWeight.bold, color: colors.onSurface),
      title: Row(
        children: [
          Icon(Icons.verified_user_outlined, color: colors.primary),
          const SizedBox(width: 10),
          Expanded(child: Text(hhText(context, '安全验证'))),
          IconButton(
            tooltip: hhText(context, '取消'),
            onPressed: () => Navigator.pop(context),
            icon: const Icon(Icons.close),
          ),
        ],
      ),
      content: SizedBox(
        width: 320,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Align(
                alignment: Alignment.centerLeft,
                child: Text(hhText(context, '拖动滑块，使拼图与缺口重合')),
              ),
              const SizedBox(height: 16),
              if (challenge != null) ...[
                ClipRRect(
                  borderRadius: BorderRadius.circular(12),
                  child: AspectRatio(
                    aspectRatio: challenge.width / challenge.height,
                    child: LayoutBuilder(
                      builder: (_, constraints) {
                        final scale = constraints.maxWidth / challenge.width;
                        return Stack(
                          children: [
                            Positioned.fill(
                              child: Image.memory(
                                challenge.background,
                                fit: BoxFit.fill,
                                gaplessPlayback: true,
                              ),
                            ),
                            Positioned(
                              left: _x * scale,
                              top: challenge.y * scale,
                              width: challenge.pieceWidth * scale,
                              height: challenge.pieceHeight * scale,
                              child: Image.memory(
                                challenge.piece,
                                fit: BoxFit.fill,
                                gaplessPlayback: true,
                              ),
                            ),
                          ],
                        );
                      },
                    ),
                  ),
                ),
                const SizedBox(height: 12),
                DecoratedBox(
                  decoration: BoxDecoration(
                    color: colors.surfaceContainerHighest,
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Slider(
                    key: const ValueKey('captcha-slider'),
                    semanticFormatterCallback: (_) =>
                        hhText(context, '拖动滑块完成验证'),
                    min: challenge.startX.toDouble(),
                    max: (challenge.width - challenge.pieceWidth).toDouble(),
                    value: _x,
                    onChanged: _checking ? null : (x) => setState(() => _x = x),
                    onChangeEnd: _checking ? null : (x) => unawaited(_solve(x)),
                  ),
                ),
              ] else if (_loading)
                const SizedBox(
                  height: 160,
                  child: Center(child: CircularProgressIndicator()),
                ),
              if (_checking) const LinearProgressIndicator(),
              if (_message != null)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Semantics(
                    liveRegion: true,
                    child: Text(hhText(context, _message!)),
                  ),
                ),
              Align(
                alignment: Alignment.centerRight,
                child: TextButton.icon(
                  key: const ValueKey('captcha-refresh'),
                  onPressed: _checking || _loading
                      ? null
                      : () => unawaited(_load()),
                  icon: const Icon(Icons.refresh, size: 18),
                  label: Text(hhText(context, '换一张')),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
