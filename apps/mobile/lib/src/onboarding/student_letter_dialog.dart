import 'package:flutter/material.dart';

import '../design_system/motion.dart';
import '../design_system/telegram_theme.dart';
import '../localization/app_localizations.dart';
import '../localization/translations_student_letter.dart';

class StudentLetterDialog extends StatefulWidget {
  const StudentLetterDialog({super.key, this.onRead});

  final Future<void> Function()? onRead;

  @override
  State<StudentLetterDialog> createState() => _StudentLetterDialogState();
}

class _StudentLetterDialogState extends State<StudentLetterDialog> {
  bool _reachedEnd = false, _saving = false, _saveFailed = false;
  double _readingProgress = 0;

  bool _observeScroll(ScrollNotification notification) {
    if (notification.depth != 0) return false;
    final metrics = notification.metrics;
    final progress = metrics.maxScrollExtent <= 0
        ? 1.0
        : (metrics.pixels / metrics.maxScrollExtent).clamp(0.0, 1.0);
    final reachedEnd = metrics.maxScrollExtent > 0 && metrics.extentAfter <= 1;
    if ((progress - _readingProgress).abs() >= .002 ||
        (reachedEnd && !_reachedEnd)) {
      setState(() {
        _readingProgress = progress;
        if (reachedEnd) _reachedEnd = true;
      });
    }
    return false;
  }

  Future<void> _acknowledge() async {
    if (!_reachedEnd || _saving) return;
    setState(() {
      _saving = true;
      _saveFailed = false;
    });
    try {
      await widget.onRead?.call();
      if (mounted && ModalRoute.isCurrentOf(context) == true) {
        Navigator.of(context).pop();
      }
    } catch (_) {
      if (mounted) {
        setState(() {
          _saving = false;
          _saveFailed = true;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final motion = HhMotion.duration(context, HhMotion.quick);
    final text = Theme.of(context).textTheme;
    return PopScope(
      canPop: false,
      child: Dialog(
        key: const ValueKey('student-letter-dialog'),
        insetPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 24),
        constraints: const BoxConstraints(maxWidth: 560),
        backgroundColor: p.raised,
        surfaceTintColor: Colors.transparent,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        clipBehavior: Clip.antiAlias,
        child: SizedBox(
          width: 560,
          height: (MediaQuery.sizeOf(context).height * .9).clamp(0.0, 760.0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _LetterHeader(
                title: hhText(context, studentLetterTitle),
                accent: p.action,
                line: p.line,
              ),
              Expanded(
                child: Stack(
                  children: [
                    Positioned.fill(
                      child: LayoutBuilder(
                        builder: (context, constraints) =>
                            NotificationListener<ScrollNotification>(
                              onNotification: _observeScroll,
                              child: SingleChildScrollView(
                                key: const ValueKey('student-letter-scroll'),
                                child: Column(
                                  crossAxisAlignment:
                                      CrossAxisAlignment.stretch,
                                  children: [
                                    // Keep the action below the first viewport
                                    // even when a wide screen fits the letter.
                                    ConstrainedBox(
                                      constraints: BoxConstraints(
                                        minHeight: constraints.maxHeight + 1,
                                      ),
                                      child: Padding(
                                        padding: const EdgeInsets.fromLTRB(
                                          24,
                                          23,
                                          24,
                                          28,
                                        ),
                                        child: DefaultTextStyle(
                                          key: const ValueKey(
                                            'student-letter-body',
                                          ),
                                          style:
                                              (text.bodyLarge ??
                                                      const TextStyle())
                                                  .copyWith(
                                                    color: p.ink,
                                                    height: 1.75,
                                                    letterSpacing: 0,
                                                  ),
                                          child: Column(
                                            crossAxisAlignment:
                                                CrossAxisAlignment.stretch,
                                            children: [
                                              for (
                                                var index = 0;
                                                index <
                                                    studentLetterParagraphs
                                                        .length;
                                                index++
                                              )
                                                Padding(
                                                  padding:
                                                      const EdgeInsets.only(
                                                        bottom: 20,
                                                      ),
                                                  child: Text(
                                                    hhText(
                                                      context,
                                                      studentLetterParagraphs[index],
                                                    ),
                                                    style: index == 0
                                                        ? (text.titleMedium ??
                                                                  const TextStyle())
                                                              .copyWith(
                                                                height: 1.6,
                                                              )
                                                        : null,
                                                  ),
                                                ),
                                              Text(
                                                hhText(
                                                  context,
                                                  studentLetterSignature,
                                                ),
                                                textAlign: TextAlign.end,
                                              ),
                                            ],
                                          ),
                                        ),
                                      ),
                                    ),
                                    Padding(
                                      padding: const EdgeInsets.fromLTRB(
                                        24,
                                        8,
                                        24,
                                        28,
                                      ),
                                      child: Column(
                                        crossAxisAlignment:
                                            CrossAxisAlignment.stretch,
                                        children: [
                                          if (_saveFailed)
                                            Padding(
                                              padding: const EdgeInsets.only(
                                                bottom: 12,
                                              ),
                                              child: Semantics(
                                                liveRegion: true,
                                                child: Text(
                                                  hhText(
                                                    context,
                                                    '阅读记录未保存，请重试。',
                                                  ),
                                                  style: TextStyle(
                                                    color: p.error,
                                                  ),
                                                ),
                                              ),
                                            ),
                                          Visibility(
                                            visible: _reachedEnd,
                                            maintainState: true,
                                            maintainAnimation: true,
                                            maintainSize: true,
                                            child: FilledButton(
                                              key: const ValueKey(
                                                'student-letter-read',
                                              ),
                                              onPressed: !_reachedEnd || _saving
                                                  ? null
                                                  : _acknowledge,
                                              style: FilledButton.styleFrom(
                                                minimumSize: const Size(0, 52),
                                              ),
                                              child: Text(
                                                hhText(context, '我已阅读'),
                                                textAlign: TextAlign.center,
                                              ),
                                            ),
                                          ),
                                        ],
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ),
                      ),
                    ),
                    Positioned(
                      key: const ValueKey('student-letter-progress'),
                      top: 8,
                      right: 4,
                      bottom: 16,
                      width: 2,
                      child: ExcludeSemantics(
                        child: ClipRRect(
                          borderRadius: BorderRadius.circular(99),
                          child: Stack(
                            fit: StackFit.expand,
                            children: [
                              ColoredBox(color: p.line.withValues(alpha: .65)),
                              Align(
                                alignment: Alignment.topCenter,
                                child: FractionallySizedBox(
                                  heightFactor: _readingProgress,
                                  widthFactor: 1,
                                  child: ColoredBox(
                                    key: const ValueKey(
                                      'student-letter-progress-fill',
                                    ),
                                    color: p.action,
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                    Positioned(
                      right: 0,
                      bottom: 8,
                      width: 24,
                      height: 24,
                      child: IgnorePointer(
                        child: ExcludeSemantics(
                          child: AnimatedOpacity(
                            key: const ValueKey('student-letter-scroll-cue'),
                            opacity: _reachedEnd ? 0 : 1,
                            duration: motion,
                            curve: HhMotion.curve,
                            child: Container(
                              decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                color: Color.alphaBlend(
                                  p.surface.withValues(alpha: .92),
                                  p.background,
                                ),
                                border: Border.all(color: p.line),
                              ),
                              child: Icon(
                                Icons.keyboard_arrow_down_rounded,
                                size: 14,
                                color: p.secondary,
                              ),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _LetterHeader extends StatelessWidget {
  const _LetterHeader({
    required this.title,
    required this.accent,
    required this.line,
  });

  final String title;
  final Color accent;
  final Color line;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.fromLTRB(24, 25, 24, 17),
    decoration: BoxDecoration(
      border: Border(bottom: BorderSide(color: line)),
    ),
    child: Stack(
      clipBehavior: Clip.none,
      alignment: Alignment.topLeft,
      children: [
        Positioned(
          top: 0,
          left: -24,
          child: Container(
            key: const ValueKey('student-letter-accent'),
            width: 3,
            height: 24,
            decoration: BoxDecoration(
              color: accent,
              borderRadius: const BorderRadius.horizontal(
                right: Radius.circular(2),
              ),
            ),
          ),
        ),
        Semantics(
          headingLevel: 1,
          child: Text(
            title,
            style: Theme.of(context).textTheme.titleLarge?.copyWith(
              color: HhTelegramPalette.of(context).ink,
              fontWeight: FontWeight.w600,
              letterSpacing: 0,
            ),
          ),
        ),
      ],
    ),
  );
}
