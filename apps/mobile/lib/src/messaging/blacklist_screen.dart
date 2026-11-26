import 'dart:async';

import 'package:flutter/material.dart';

import '../design_system/motion_routes.dart';
import '../design_system/feedback.dart';
import '../design_system/telegram_theme.dart';
import '../identity/identity_avatar.dart';
import '../localization/app_localizations.dart';
import '../posts/author_block_api.dart';
import '../posts/post_controller.dart';
import '../posts/post_models.dart';
import '../posts/x_feed.dart';

/// Shows the current account's author blocks without revealing linked identities.
class DmBlacklistScreen extends StatefulWidget {
  const DmBlacklistScreen({super.key, required this.posts});

  final PostController posts;

  @override
  State<DmBlacklistScreen> createState() => _DmBlacklistScreenState();
}

class _DmBlacklistScreenState extends State<DmBlacklistScreen> {
  late final _posts = widget.posts;
  late final _authority = _posts.authorityVersion;
  List<BlockedAuthor>? _authors;
  bool _loading = false;
  String? _pending;
  bool _removing = false;
  String? _error;

  bool get _current =>
      identical(_posts, widget.posts) &&
      _posts.supportsAuthorBlock &&
      _posts.available &&
      _posts.authorityVersion == _authority;

  @override
  void initState() {
    super.initState();
    // Bind this route to the account window in which it was opened.
    _authority;
    unawaited(_load());
  }

  Future<void> _load() async {
    if (!_current || _loading || _pending != null) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final authors = await _posts.blockedAuthors();
      if (mounted && _current) setState(() => _authors = authors);
    } catch (_) {
      if (mounted && _current) {
        setState(() => _error = '黑名单读取失败，请重试');
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _unblock(BlockedAuthor item) async {
    if (!_current || _loading || _pending != null) return;
    setState(() => _pending = item.blockId);
    try {
      final p = HhTelegramPalette.of(context);
      final confirmed = await showModalBottomSheet<bool>(
        context: context,
        useSafeArea: true,
        isScrollControlled: true,
        backgroundColor: p.raised,
        sheetAnimationStyle: HhMotionRoutes.sheet(context),
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
        ),
        builder: (context) => AnimatedBuilder(
          animation: _posts,
          builder: (context, _) =>
              _UnblockSheet(author: item.author, current: _current),
        ),
      );
      if (confirmed != true || !mounted || !_current) return;
      setState(() => _removing = true);
      final removed = await _posts.unblockAuthor(item.blockId);
      if (!mounted || !_current) return;
      showHhFeedback(
        context,
        removed ? '已解除拉黑' : _posts.authorBlockError ?? '操作未保存，请重试',
      );
      if (!removed) return;
      setState(() {
        _authors = [
          for (final author in _authors ?? const <BlockedAuthor>[])
            if (author.blockId != item.blockId) author,
        ];
        _pending = null;
        _removing = false;
      });
      await _load();
      if (mounted && _current) unawaited(_posts.reloadVisibleFeeds());
    } finally {
      if (mounted) {
        setState(() {
          _pending = null;
          _removing = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _posts,
    builder: (context, _) {
      final p = HhTelegramPalette.of(context);
      return Scaffold(
        backgroundColor: p.surface,
        appBar: AppBar(
          automaticallyImplyLeading: false,
          centerTitle: false,
          titleSpacing: 24,
          backgroundColor: p.surface,
          surfaceTintColor: Colors.transparent,
          title: Text(
            hhText(context, '黑名单'),
            style: TextStyle(
              color: p.ink,
              fontSize: 22,
              fontWeight: FontWeight.w700,
              height: 1.35,
            ),
          ),
          bottom: PreferredSize(
            preferredSize: const Size.fromHeight(1),
            child: Divider(height: 1, color: p.line),
          ),
        ),
        body: SafeArea(
          top: false,
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 640),
              child: Column(
                children: [
                  SizedBox(
                    height: 2,
                    child: _current && _loading && _authors != null
                        ? const LinearProgressIndicator(minHeight: 2)
                        : null,
                  ),
                  Expanded(
                    child: !_current
                        ? _ListNotice(
                            icon: Icons.lock_outline,
                            text: hhText(context, '会话已变化，请重新进入'),
                          )
                        : _content(p),
                  ),
                ],
              ),
            ),
          ),
        ),
      );
    },
  );

  Widget _content(HhTelegramPalette p) {
    final authors = _authors;
    if (authors == null && _loading) return const _LoadingRows();
    return RefreshIndicator(
      onRefresh: _load,
      child: CustomScrollView(
        key: const ValueKey('blacklist-scroll'),
        physics: const AlwaysScrollableScrollPhysics(),
        slivers: [
          if (_error != null && authors?.isNotEmpty == true)
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(24, 12, 24, 0),
                child: TextButton.icon(
                  onPressed: _loading || _pending != null ? null : _load,
                  icon: const Icon(Icons.refresh, size: 20),
                  label: Text(hhText(context, _error!)),
                  style: TextButton.styleFrom(
                    foregroundColor: p.error,
                    alignment: Alignment.centerLeft,
                    minimumSize: const Size.fromHeight(48),
                  ),
                ),
              ),
            ),
          if (_error != null && (authors?.isEmpty ?? true))
            SliverFillRemaining(
              hasScrollBody: false,
              child: _ListNotice(
                icon: Icons.cloud_off_outlined,
                text: hhText(context, _error!),
                onRetry: _loading ? null : _load,
              ),
            )
          else if (authors?.isNotEmpty != true)
            const SliverFillRemaining(
              hasScrollBody: false,
              child: _EmptyState(),
            )
          else
            SliverPadding(
              padding: const EdgeInsets.only(top: 8, bottom: 24),
              sliver: SliverList.separated(
                itemCount: authors!.length,
                separatorBuilder: (_, _) => Divider(
                  height: 1,
                  indent: 88,
                  endIndent: 24,
                  color: p.line,
                ),
                itemBuilder: (context, index) {
                  final item = authors[index];
                  final name = localizedPostAuthor(context, item.author);
                  final removing = _pending == item.blockId && _removing;
                  return ConstrainedBox(
                    key: ValueKey('author-block:${item.blockId}'),
                    constraints: const BoxConstraints(minHeight: 88),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 24,
                        vertical: 16,
                      ),
                      child: Row(
                        children: [
                          _BlockAvatar(author: item.author, name: name),
                          const SizedBox(width: 16),
                          Expanded(
                            child: Text(
                              name,
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 16,
                                fontWeight: FontWeight.w500,
                                height: 1.4,
                                color: p.ink,
                              ),
                            ),
                          ),
                          const SizedBox(width: 16),
                          Semantics(
                            label: name,
                            child: OutlinedButton(
                              key: ValueKey('author-unblock:${item.blockId}'),
                              onPressed: _pending != null || _loading
                                  ? null
                                  : () => unawaited(_unblock(item)),
                              style: OutlinedButton.styleFrom(
                                minimumSize: const Size(64, 48),
                                foregroundColor: p.ink,
                                side: BorderSide(color: p.secondary),
                                shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(8),
                                ),
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 12,
                                  vertical: 10,
                                ),
                              ),
                              child: Stack(
                                alignment: Alignment.center,
                                children: [
                                  Visibility(
                                    visible: !removing,
                                    maintainState: true,
                                    maintainAnimation: true,
                                    maintainSize: true,
                                    child: Text(hhText(context, '解除')),
                                  ),
                                  if (removing)
                                    const SizedBox.square(
                                      dimension: 18,
                                      child: CircularProgressIndicator(
                                        strokeWidth: 2,
                                      ),
                                    ),
                                ],
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  );
                },
              ),
            ),
        ],
      ),
    );
  }
}

class _BlockAvatar extends StatelessWidget {
  const _BlockAvatar({
    required this.author,
    required this.name,
    this.radius = 24,
  });

  final PostAuthor author;
  final String name;
  final double radius;

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final tone = switch (name.isEmpty
        ? 0
        : name.codeUnits.fold(0, (a, b) => a + b) % 3) {
      0 => hhQuietAction(context),
      1 => const Color(0xFFB18A62),
      _ => const Color(0xFFB45F68),
    };
    final toneOpacity = Theme.of(context).brightness == Brightness.dark
        ? .18
        : .13;
    return ExcludeSemantics(
      child: IdentityAvatar(
        avatar: author.avatar,
        radius: radius,
        fallback: CircleAvatar(
          radius: radius,
          backgroundColor: Color.alphaBlend(
            tone.withValues(alpha: toneOpacity),
            p.background,
          ),
          foregroundColor: p.ink,
          child: Text(
            name.isEmpty ? '?' : name.characters.first,
            textScaler: TextScaler.noScaling,
            style: TextStyle(fontSize: radius * .8),
          ),
        ),
      ),
    );
  }
}

class _EmptyState extends StatelessWidget {
  const _EmptyState();

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return Align(
      alignment: const Alignment(0, -.08),
      child: Padding(
        padding: const EdgeInsets.all(40),
        child: Column(
          key: const ValueKey('blacklist-empty'),
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 64,
              height: 64,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: p.background,
                border: Border.all(color: p.line),
              ),
              child: Icon(
                Icons.person_off_outlined,
                size: 28,
                color: p.secondary,
              ),
            ),
            const SizedBox(height: 18),
            Text(
              hhText(context, '暂无记录'),
              textAlign: TextAlign.center,
              style: TextStyle(
                color: p.ink,
                fontSize: 16,
                fontWeight: FontWeight.w500,
                height: 1.5,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ListNotice extends StatelessWidget {
  const _ListNotice({required this.icon, required this.text, this.onRetry});

  final IconData icon;
  final String text;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ExcludeSemantics(child: Icon(icon, size: 40, color: p.secondary)),
            const SizedBox(height: 20),
            if (onRetry != null)
              TextButton.icon(
                onPressed: onRetry,
                icon: const Icon(Icons.refresh, size: 20),
                label: Text(text, textAlign: TextAlign.center),
                style: TextButton.styleFrom(
                  foregroundColor: p.ink,
                  minimumSize: const Size(48, 48),
                ),
              )
            else
              Text(
                text,
                textAlign: TextAlign.center,
                style: TextStyle(color: p.ink, fontSize: 16, height: 1.5),
              ),
          ],
        ),
      ),
    );
  }
}

class _LoadingRows extends StatelessWidget {
  const _LoadingRows();

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    return ExcludeSemantics(
      child: ListView.builder(
        key: const ValueKey('blacklist-loading'),
        padding: const EdgeInsets.only(top: 8),
        itemCount: 5,
        itemBuilder: (context, index) => Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 20),
          child: Row(
            children: [
              CircleAvatar(radius: 24, backgroundColor: p.background),
              const SizedBox(width: 16),
              Expanded(
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: FractionallySizedBox(
                    widthFactor: index.isEven ? .65 : .45,
                    child: Container(height: 14, color: p.background),
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

class _UnblockSheet extends StatelessWidget {
  const _UnblockSheet({required this.author, required this.current});

  final PostAuthor author;
  final bool current;

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final action = hhQuietAction(context);
    final onAction = hhOnQuietAction(context);
    final name = localizedPostAuthor(context, author);
    return SingleChildScrollView(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 10, 24, 12),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ExcludeSemantics(
              child: Center(
                child: Container(
                  key: const ValueKey('author-unblock-handle'),
                  width: 38,
                  height: 4,
                  margin: const EdgeInsets.only(bottom: 18),
                  decoration: BoxDecoration(
                    color: p.line,
                    borderRadius: BorderRadius.circular(99),
                  ),
                ),
              ),
            ),
            if (current) ...[
              Center(
                child: _BlockAvatar(author: author, name: name, radius: 28),
              ),
              const SizedBox(height: 12),
              Text(
                name,
                textAlign: TextAlign.center,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w500,
                  height: 1.4,
                  color: p.ink,
                ),
              ),
            ] else
              Text(hhText(context, '会话已变化，请重新进入'), textAlign: TextAlign.center),
            const SizedBox(height: 24),
            FilledButton(
              key: const ValueKey('author-unblock-confirm'),
              onPressed: current ? () => Navigator.pop(context, true) : null,
              style: FilledButton.styleFrom(
                minimumSize: const Size.fromHeight(48),
                padding: const EdgeInsets.symmetric(vertical: 14),
                backgroundColor: action,
                foregroundColor: onAction,
                disabledBackgroundColor: action.withValues(alpha: .45),
                disabledForegroundColor: onAction.withValues(alpha: .72),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(8),
                ),
              ),
              child: Text(hhText(context, '解除拉黑')),
            ),
            const SizedBox(height: 4),
            TextButton(
              key: const ValueKey('author-unblock-cancel'),
              onPressed: () => Navigator.pop(context),
              style: TextButton.styleFrom(
                minimumSize: const Size.fromHeight(48),
                foregroundColor: p.ink,
              ),
              child: Text(hhText(context, '取消')),
            ),
          ],
        ),
      ),
    );
  }
}
