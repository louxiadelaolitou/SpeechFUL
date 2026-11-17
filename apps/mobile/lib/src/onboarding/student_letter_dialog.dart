import 'package:flutter/material.dart';

import '../design_system/motion.dart';
import '../design_system/telegram_theme.dart';
import '../localization/app_localizations.dart';
import '../localization/translations_student_letter.dart';

Future<void> showStudentLetter(BuildContext context) => showDialog<void>(
  context: context,
  barrierDismissible: false,
  builder: (_) => const StudentLetterDialog(),
);

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
    final action = hhQuietAction(context);
    final onAction = hhOnQuietAction(context);
    final motion = HhMotion.duration(context, HhMotion.quick);
    final buttonTextStyle =
        (Theme.of(context).textTheme.labelLarge ?? const TextStyle()).copyWith(
          fontSize: 15,
          fontWeight: FontWeight.w600,
        );
    return PopScope(
      canPop: false,
      child: Dialog(
        key: const ValueKey('student-letter-dialog'),
        insetPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 24),
        constraints: const BoxConstraints(maxWidth: 560),
        backgroundColor: p.surface,
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
                accent: action,
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
                                          style: TextStyle(
                                            color: p.ink,
                                            fontFamily: 'StudentLetterSerif',
                                            fontFamilyFallback: const [
                                              'HhNotoSansSC',
                                            ],
                                            fontSize: 16,
                                            height: 1.85,
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
                                                        ? const TextStyle(
                                                            fontSize: 17,
                                                            height: 1.7,
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
                                                padding: const EdgeInsets.all(
                                                  16,
                                                ),
                                                backgroundColor: action,
                                                foregroundColor: onAction,
                                                disabledBackgroundColor: action
                                                    .withValues(alpha: .45),
                                                disabledForegroundColor:
                                                    onAction.withValues(
                                                      alpha: .72,
                                                    ),
                                                textStyle: buttonTextStyle,
                                                shape: RoundedRectangleBorder(
                                                  borderRadius:
                                                      BorderRadius.circular(8),
                                                ),
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
                                  child: ColoredBox(color: action),
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
            style: TextStyle(
              color: HhTelegramPalette.of(context).ink,
              fontSize: 20,
              fontWeight: FontWeight.w700,
              height: 1.4,
              letterSpacing: 0,
            ),
          ),
        ),
      ],
    ),
  );
}
