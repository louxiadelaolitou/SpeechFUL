import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:hnuhole_auth_passkey/hnuhole_auth_passkey.dart';

import 'hnuhole_mobile.dart';
import 'src/system_notifications/inbox_screen.dart';
import 'src/comments/comment_runtime.dart';
import 'src/comments/comment_http.dart';
import 'src/comments/comment_journal.dart';
import 'src/development/app_transport.dart';
import 'src/navigation/primary_tabs.dart';
import 'src/design_system/motion.dart';
import 'src/localization/app_localizations.dart';
import 'src/auth/device_management_controller.dart';
import 'src/auth/device_management_screen.dart';
import 'src/auth/account_closure_screen.dart';
import 'src/navigation/community_material_labels.dart';
import 'src/navigation/app_navigation.dart';
import 'src/messaging/direct_messages_unavailable.dart';
import 'src/messaging/dm_runtime.dart';
import 'src/messaging/dm_controller.dart';
import 'src/messaging/dm_store.dart';
import 'src/messaging/dm_notifications.dart';
import 'src/messaging/dm_governance_ui.dart';
import 'src/messaging/direct_message_models.dart';
import 'src/design_system/telegram_theme.dart';
import 'src/identity/identity_initial_gate.dart';
import 'src/identity/identity_avatar.dart';
import 'src/identity/identity_avatar_flow.dart';
import 'src/identity/active_identity_controller.dart';

Uri _communityBaseUri() => Uri.parse(
  const String.fromEnvironment(
    'AUTH_COMMUNITY_BASE_URL',
    defaultValue: 'https://community.hnuhole.invalid',
  ),
);

class _AppTextScaler extends TextScaler {
  const _AppTextScaler(this.platform, this.factor);

  final TextScaler platform;
  final double factor;

  @override
  double scale(double fontSize) => platform.scale(fontSize) * factor;

  @override
  double get textScaleFactor => scale(14) / 14;

  @override
  bool operator ==(Object other) =>
      other is _AppTextScaler &&
      other.platform == platform &&
      other.factor == factor;

  @override
  int get hashCode => Object.hash(platform, factor);
}

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final postLinks = PostLinkInbox(_communityBaseUri())..start();
  HhTelegramTheme.registerFontLicenses();
  final presentation = AuthPresentationController(
    preferences: FlutterAuthVault(namespace: 'hnuhole.presentation.v1'),
  );
  try {
    await Future.wait([_preloadWelcomeMark(), presentation.load()]);
  } catch (error, stackTrace) {
    FlutterError.reportError(
      FlutterErrorDetails(
        exception: error,
        stack: stackTrace,
        library: 'app startup',
        context: ErrorDescription('while preparing the welcome mark'),
      ),
    );
  }
  runApp(_StartupWelcomeApp(presentation));
  await WidgetsBinding.instance.endOfFrame;
  _runHnuholeApp(presentation, postLinks);
}

Future<void> _preloadWelcomeMark() async {
  final view = WidgetsBinding.instance.platformDispatcher.views.first;
  final stream = const AssetImage('assets/branding/welcome-shush-mark.png')
      .resolve(
        ImageConfiguration(
          bundle: rootBundle,
          devicePixelRatio: view.devicePixelRatio,
        ),
      );
  final ready = Completer<void>();
  late final ImageStreamListener listener;
  listener = ImageStreamListener(
    (_, _) {
      stream.removeListener(listener);
      if (!ready.isCompleted) ready.complete();
    },
    onError: (Object error, StackTrace? stackTrace) {
      stream.removeListener(listener);
      if (!ready.isCompleted) ready.completeError(error, stackTrace);
    },
  );
  stream.addListener(listener);
  await ready.future;
}

/// Let the welcome page become Flutter's first frame before constructing the
/// runtime graph, so cold start keeps the same branding while session restore runs.
class _StartupWelcomeApp extends StatelessWidget {
  const _StartupWelcomeApp(this.presentation);

  final AuthPresentationController presentation;

  @override
  Widget build(BuildContext context) => MaterialApp(
    initialRoute: '/',
    debugShowCheckedModeBanner: false,
    themeMode: presentation.themeMode,
    locale: presentation.language.locale,
    supportedLocales: appSupportedLocales,
    localizationsDelegates: appLocalizationDelegates,
    builder: (context, child) => AuthPresentationScope(
      controller: presentation,
      child: child ?? const SizedBox.shrink(),
    ),
    theme: HhTelegramTheme.applyAppearance(ThemeData(useMaterial3: true)),
    darkTheme: HhTelegramTheme.applyAppearance(
      ThemeData(useMaterial3: true, brightness: Brightness.dark),
    ),
    home: const AuthWelcomeScreen(
      onStarted: _ignoreStartupAction,
      showControls: false,
    ),
  );
}

void _ignoreStartupAction() {}

void _runHnuholeApp(
  AuthPresentationController presentation,
  PostLinkInbox postLinks,
) {
  final communityBaseUri = _communityBaseUri();
  final verifierBaseUri = Uri.parse(
    const String.fromEnvironment(
      'AUTH_VERIFIER_BASE_URL',
      defaultValue: 'https://verifier.hnuhole.invalid',
    ),
  );
  final environment = MobileEnvironment(
    communityBaseUri: communityBaseUri,
    verifierBaseUri: verifierBaseUri,
  );
  final repository = HttpChannelRepository(
    baseUri: communityBaseUri,
    client: createAppHttpClient(
      community: communityBaseUri,
      verifier: verifierBaseUri,
    ),
  );
  final directory = ChannelDirectoryController(repository: repository);
  final api = HttpAuthApi(
    communityBaseUri: communityBaseUri,
    verifierBaseUri: verifierBaseUri,
    passkeyRpId: developmentPasskeyRp(
      community: communityBaseUri,
      verifier: verifierBaseUri,
    ),
    client: createAppHttpClient(
      community: communityBaseUri,
      verifier: verifierBaseUri,
    ),
  );
  final storageScope = environment.storageScope;
  final storageNamespace = AuthCrypto.domainDigest(
    'HNUHOLE/MOBILE-AUTH-ENVIRONMENT/V1',
    utf8.encode(storageScope),
  );
  final store = AuthStore(
    FlutterAuthVault(namespace: 'hnuhole.auth.v1.$storageNamespace'),
    scope: storageScope,
  );
  final sessions = AuthSessionController(
    api: api,
    store: store,
    directory: directory,
  );
  final postApi = HttpPostUploadApi(
    communityBaseUri: communityBaseUri,
    client: createAppHttpClient(
      community: communityBaseUri,
      verifier: verifierBaseUri,
    ),
  );
  final posts = PostController(
    channelDirectory: () => directory.channels,
    api: postApi,
    mediaApi: postApi,
    uploadApi: postApi,
    photoPicker: NativePostPhotoPicker(),
    sessions: sessions,
    authStore: store,
    identities: api,
    openStore: (accountId) =>
        PostStore.openNative(environment: storageScope, accountId: accountId),
    purgeStore: (accountId) => PostStore.purgeNativeClosedAccount(
      environment: storageScope,
      accountId: accountId,
    ),
  );
  final postMedia = PostMediaRepository(posts);
  final governanceApi = HttpGovernanceApi(
    baseUri: communityBaseUri,
    client: createAppHttpClient(
      community: communityBaseUri,
      verifier: verifierBaseUri,
    ),
  );
  final moderationAccess = ModerationAccess(
    api: governanceApi,
    vault: FlutterAuthVault(
      namespace: 'hnuhole.moderation-access.v1.$storageNamespace',
    ),
  );
  final governance = GovernanceController(
    api: governanceApi,
    sessions: sessions,
    authStore: store,
    identitiesApi: api,
    restrictedAccess: moderationAccess,
    pendingStore: GovernancePendingStore(
      GovernanceMaterialVault(
        openStore: () => PostStore.openNative(
          environment: '$storageScope|governance-material-v1',
          accountId: 'governance-pending-owners',
        ),
      ),
    ),
  );
  final passkey = NativePasskeyClient();
  // Legacy credentials remain stored while all recovery ceremonies are paused.
  final management = SecurityManagementController(
    legacyRecoveryEnabled: false,
    api: api,
    store: store,
    passkey: passkey,
    sessions: sessions,
  );
  final identities = IdentityManagementController(
    api: api,
    store: store,
    sessions: sessions,
  );
  final comments = CommentRuntime(
    http: CommentHttp(
      communityBaseUri,
      client: createAppHttpClient(
        community: communityBaseUri,
        verifier: verifierBaseUri,
      ),
    ),
    sessions: sessions,
    authStore: store,
    identities: api,
    identityManagement: identities,
    journal: NativeCommentJournal(
      (account) => PostStore.openNative(
        environment: '$storageScope|comments-v1',
        accountId: account,
      ),
      purgeStore: (account) => PostStore.purgeNativeClosedAccount(
        environment: '$storageScope|comments-v1',
        accountId: account,
      ),
    ),
  );
  comments.openRetarget = (context, input, bytes) =>
      Navigator.of(context).push<void>(
        MaterialPageRoute(
          builder: (_) => PostDetailScreen(
            controller: posts,
            postId: input.binding.postId,
            loadImages: postMedia.load,
            initialCommentInput: input,
            initialCommentOriginal: bytes,
          ),
        ),
      );
  late final DmRuntime dm;
  final dmNotifications = DmNotifications(
    environment: storageScope,
    preferences: FlutterAuthVault(
      namespace: 'hnuhole.dm.notification.$storageNamespace',
    ),
  );
  final flows = AuthFlows(
    legacyRecoveryEnabled: false,
    api: api,
    store: store,
    acceptRegistrationSession: sessions.acceptSession,
    clearCommunityAccess: sessions.clearCommunityAccess,
    passkeyApi: api,
    passkey: passkey,
    sessionAuthorityVersion: () => sessions.authorityVersion,
    sessionAuthority: sessions,
    onAccountClosed: (accountId) async {
      await dm.purgeClosedAccount(accountId);
      await dmNotifications.purge(accountId);
      await posts.purgeClosedAccount(accountId);
      await comments.purgeClosedAccount(accountId);
      await governance.purgeClosedAccount(accountId);
    },
  );

  dm = DmRuntime(
    controller: DmController(
      chatEnabled: true,
      api: postApi,
      openStore: (account) => DmStore.openNative(storageScope, account),
      purgeStore: (account) => DmStore.purgeNativeClosedAccount(
        environment: storageScope,
        accountId: account,
      ),
      onIncoming: (items, account) =>
          dmNotifications.incoming(items, account, dm.controller),
      onUnauthorized: sessions.sessionUnauthorized,
      onUnavailable: sessions.sessionUnavailable,
      onMetadata: (token, expires, current) async {
        await sessions.persistSessionMetadata(token, expires, current);
      },
    ),
    sessions: sessions,
    authStore: store,
    identityApi: api,
    identityChanges: identities,
    openPost: (context, id) => Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => PostDetailScreen(
          controller: posts,
          postId: id,
          loadImages: postMedia.load,
        ),
      ),
    ),
  );
  runApp(
    HnuholeApp(
      postLinks: postLinks,
      presentation: presentation,
      directory: directory,
      repository: repository,
      api: api,
      sessions: sessions,
      flows: flows,
      management: management,
      identities: identities,
      store: store,
      posts: posts,
      postApi: postApi,
      postMedia: postMedia,
      loadPostImages: postMedia.load,
      comments: comments,
      governance: governance,
      governanceApi: governanceApi,
      dm: dm,
      dmNotifications: dmNotifications,
    ),
  );
}

/// The production root used by main and by the real-transport integration test.
/// Dependencies remain the shipped controllers/adapters; native ports are
/// replaced only in the host fixture, whose proof does not cover devices.
class HnuholeApp extends StatefulWidget {
  const HnuholeApp({
    super.key,
    this.presentation,
    this.postLinks,
    this.sharePostText,
    required this.directory,
    required this.repository,
    required this.api,
    required this.sessions,
    required this.flows,
    required this.management,
    required this.identities,
    required this.store,
    required this.posts,
    required this.postApi,
    this.loadPostImages,
    this.postMedia,
    this.governance,
    this.governanceApi,
    this.comments,
    this.dm,
    this.dmNotifications,
    this.studentLetters,
  });

  final ChannelDirectoryController directory;
  final PostLinkInbox? postLinks;
  final PostShareSender? sharePostText;
  final HttpChannelRepository repository;
  final HttpAuthApi api;
  final AuthSessionController sessions;
  final AuthFlows flows;
  final SecurityManagementController management;
  final IdentityManagementController identities;
  final AuthStore store;
  final PostController posts;
  final HttpPostApi postApi;
  final PostImagesLoader? loadPostImages;
  final PostMediaRepository? postMedia;
  final GovernanceController? governance;
  final CommentRuntime? comments;
  final HttpGovernanceApi? governanceApi;
  final DmRuntime? dm;
  final DmNotifications? dmNotifications;
  final AuthPresentationController? presentation;
  final StudentLetterStore? studentLetters;

  @override
  State<HnuholeApp> createState() => _HnuholeAppState();
}

class _HnuholeAppState extends State<HnuholeApp>
    with WidgetsBindingObserver, SingleTickerProviderStateMixin {
  late final _studentLetters =
      widget.studentLetters ??
      StudentLetterStore.native(environment: widget.store.scope);
  late final ActiveIdentityController _activeIdentity;
  late final _authPresentation =
      widget.presentation ??
      AuthPresentationController(
        preferences: FlutterAuthVault(namespace: 'hnuhole.presentation.v1'),
      );
  final _navigatorKey = GlobalKey<NavigatorState>();
  late final PostLinkInbox _postLinks;
  bool _postLinkQueued = false;
  int? _promptedPostLink, _openingPostLink;
  int _lastLogoutVersion = 0;

  void _schedulePostLink() {
    if (_postLinkQueued || !mounted) return;
    _postLinkQueued = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _postLinkQueued = false;
      if (mounted) _openPendingPostLink();
    });
    WidgetsBinding.instance.ensureVisualUpdate();
  }

  void _openPendingPostLink() {
    final navigator = _navigatorKey.currentState;
    if (navigator == null) return;
    if (_postLinks.invalidLink) {
      _postLinks.invalidLink = false;
      final context = navigator.context;
      ScaffoldMessenger.maybeOf(context)
          ?.showSnackBar(SnackBar(content: Text(hhText(context, '分享链接无效'))));
    }
    final link = _postLinks.pending;
    if (link == null) return;
    if (!widget.sessions.isAuthenticated) {
      if (widget.sessions.status == AuthStatus.starting ||
          widget.sessions.status == AuthStatus.restoring) {
        return;
      }
      if (!_authRouteOpen && _promptedPostLink != link.sequence) {
        _promptedPostLink = link.sequence;
        _openAuth();
      }
      return;
    }
    if (_authRouteOpen ||
        !widget.identities.entryReady ||
        !widget.posts.available ||
        widget.directory.status != ChannelDirectoryStatus.ready ||
        _openingPostLink == link.sequence) {
      return;
    }
    _openingPostLink = link.sequence;
    final authority = widget.posts.authorityVersion;
    final route = MaterialPageRoute<void>(
      settings: RouteSettings(name: 'shared-post:${link.target.postId}'),
      builder: (_) => PostDetailScreen(
        controller: widget.posts,
        postId: link.target.postId,
        sharedUrl: link.target.url,
        loadImages: widget.loadPostImages,
        onLoaded: () => _postLinks.complete(link.sequence),
        onAuthenticationRequired: () {
          _postLinks.receive(link.target.url);
          _openAuth();
        },
      ),
    );
    unawaited(
      navigator.push(route).whenComplete(() {
        if (!mounted) return;
        if (_openingPostLink == link.sequence) _openingPostLink = null;
        // Back is an explicit cancellation of opening this target. Auth still
        // retains it while it removes protected routes on successful login.
        if (!_authRouteOpen &&
            widget.posts.available &&
            widget.posts.authorityVersion == authority) {
          _postLinks.complete(link.sequence);
        }
        _schedulePostLink();
      }),
    );
  }

  bool _authRouteOpen = false;
  bool _authCompletionPending = false, _authCompletionScheduled = false;
  void _scheduleAuthCompletion() {
    if (!_authCompletionPending || _authCompletionScheduled || !mounted) return;
    _authCompletionScheduled = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _authCompletionScheduled = false;
      if (!mounted || !_authCompletionPending) return;
      final sessions = widget.sessions;
      if (!sessions.isAuthenticated) {
        _authCompletionPending = false;
        return;
      }
      final ready =
          widget.posts.available &&
          widget.directory.status == ChannelDirectoryStatus.ready;
      final failed =
          !sessions.busy &&
          widget.directory.status == ChannelDirectoryStatus.failure;
      if (!ready && !failed) return;
      final navigator = _navigatorKey.currentState;
      if (navigator == null) return;
      _authCompletionPending = false;
      navigator.popUntil((route) => route.isFirst);
    });
    WidgetsBinding.instance.ensureVisualUpdate();
  }

  bool _securityRouteOpen = false;
  bool _devicesRouteOpen = false, _closureRouteOpen = false;
  bool _identityRouteOpen = false;
  bool _settingsRouteOpen = false;
  final _primaryTabs = GlobalKey<PrimaryTabsState>();
  late final _avatars = IdentityAvatarRepository(
    widget.api,
    widget.sessions,
    widget.store,
  );
  final _postWindow = ValueNotifier<(String?, int)>((null, -1));
  // Navigation preference is local to this app run and to its owning account.
  final _homeChannels = <String, String?>{};
  late final Listenable _postRouteChanges;
  bool _messagesRouteOpen = false;
  late final ValueNotifier<HhNavigationUnread> _navigationUnread;
  int _notificationUnreadAuthority = -1;
  String? _dmInitialIdentity;
  int _dmInboxRevision = 0;
  bool _handlingDmTap = false;
  Future<void> _dmNotificationChanged() async {
    final n = widget.dmNotifications, c = widget.dm?.controller;
    if (n == null || c == null || !mounted) return;
    final window = c.scope;
    await n.bind(
      c.available ? c.accountId : null,
      current: () => mounted && c.scope == window,
    );
    await n.reconcile(c);
    if (!mounted || _handlingDmTap || n.pending == null) return;
    final target = n.pending!;
    final account = widget.store.current?.session?.accountId;
    if (account != null && target.scope != n.scopeFor(account)) {
      n.pending = null;
      return;
    }
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    if (!widget.identities.entryReady || !c.available || c.accountId == null) {
      return;
    }
    if (target.scope != n.scopeFor(c.accountId!)) {
      n.pending = null;
      return;
    }
    final row = c.conversations
        .where(
          (r) =>
              r['id'] == target.conversation &&
              r['identityId'] == target.identity,
        )
        .firstOrNull;
    final own = widget.dm!.identities
        .where((i) => i.id == target.identity)
        .firstOrNull;
    // A cold local directory may still be waiting for its first server sync.
    if (row == null || own == null) return;
    if (row['deleted'] == true) {
      n.pending = null;
      return;
    }
    _handlingDmTap = true;
    n.pending = null;
    final navigator = _navigatorKey.currentState;
    if (navigator != null) {
      navigator.popUntil((r) => r.isFirst);
      setState(() {
        _dmInitialIdentity = own.id;
        _dmInboxRevision++;
      });
      _primaryTabs.currentState?.select(AppDestination.directMessages);
      unawaited(
        navigator.push(
          MaterialPageRoute<void>(
            builder: (_) => DmChatRoute(
              runtime: widget.dm!,
              own: own,
              peer: DmIdentity(
                id: row['peerId'],
                name: row['peerName'],
                avatar: row['peerAvatar'] as String? ?? 'default-v1',
              ),
              sourcePost: row['sourcePostId'],
              conversation: row['id'],
            ),
          ),
        ),
      );
    }
    _handlingDmTap = false;
    if (mounted && n.pending != null) _scheduleDmNotification();
  }

  void _scheduleDmNotification() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_dmNotificationChanged());
    });
    WidgetsBinding.instance.ensureVisualUpdate();
  }

  void _syncNavigationUnread() {
    final governance = widget.governance;
    if (governance == null || !governance.available) {
      _notificationUnreadAuthority = -1;
    } else if (_notificationUnreadAuthority != governance.authorityVersion) {
      _notificationUnreadAuthority = governance.authorityVersion;
      unawaited(governance.refreshInboxUnread());
    }
    final conversations = widget.dm?.controller.conversations ?? const [];
    final next = (
      messages: widget.governance?.hasUnread == true,
      directMessages: conversations.any((row) {
        final unread = row['unread'];
        return unread is int && unread > 0;
      }),
    );
    if (next != _navigationUnread.value) _navigationUnread.value = next;
  }

  void _selectPrimary(AppDestination destination) {
    switch (destination) {
      case AppDestination.home:
        _primaryTabs.currentState?.select(destination);
      case AppDestination.messages:
        unawaited(_openMessages());
      case AppDestination.directMessages:
        unawaited(_openDirectMessages());
      case AppDestination.personal:
        _openPersonal();
    }
  }

  Future<void> _openDirectMessages() async {
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    _primaryTabs.currentState?.select(AppDestination.directMessages);
  }

  Future<void> _openMessages() async {
    final navigator = _navigatorKey.currentState,
        governance = widget.governance;
    if (navigator == null || governance == null || _messagesRouteOpen) return;
    _messagesRouteOpen = true;
    try {
      if (!governance.available) {
        final access = governance.restrictedAccess;
        if (access == null) {
          _openAuth();
          return;
        }
        final result = await navigator.push<bool>(
          MaterialPageRoute(
            builder: (_) => ModerationAccessScreen(access: access),
          ),
        );
        if (result != true || !mounted || !governance.available) return;
      }
      final tabs = _primaryTabs.currentState;
      if (tabs != null) {
        tabs.select(AppDestination.messages);
        navigator.popUntil((route) => route.isFirst);
      } else {
        await navigator.push(
          MaterialPageRoute<void>(builder: (_) => _notificationsScreen()),
        );
      }
    } finally {
      _messagesRouteOpen = false;
    }
  }

  Widget _notificationsScreen() {
    final governance = widget.governance!;
    if (governance.restricted) {
      return SystemNotificationsScreen(controller: governance);
    }
    final navigator = _navigatorKey.currentState!;
    final owner = governance.accountId, authority = governance.authorityVersion;
    return NotificationInboxScreen(
      controller: governance,
      loadImages: widget.loadPostImages,
      onManageIdentities: () => _openIdentity(initialCreate: true),
      onOpenComment: (postId, commentId) async {
        if (!widget.posts.available ||
            governance.restricted ||
            governance.accountId != owner ||
            governance.authorityVersion != authority) {
          return;
        }
        await navigator.push(
          MaterialPageRoute<void>(
            builder: (_) => PostDetailScreen(
              controller: widget.posts,
              postId: postId,
              initialCommentId: commentId,
              loadImages: widget.loadPostImages,
              onAuthenticationRequired: _openAuth,
            ),
          ),
        );
      },
      onOpenPost: (postId) async {
        if (!widget.posts.available ||
            governance.accountId != owner ||
            governance.authorityVersion != authority) {
          return;
        }
        await navigator.push(
          MaterialPageRoute<void>(
            builder: (_) => PostDetailScreen(
              controller: widget.posts,
              postId: postId,
              loadImages: widget.loadPostImages,
              onAuthenticationRequired: _openAuth,
            ),
          ),
        );
      },
    );
  }

  @override
  void initState() {
    super.initState();
    _postLinks =
        widget.postLinks ?? PostLinkInbox(widget.postApi.communityBaseUri);
    // External links are deferred for Android V1; explicit legacy fixtures remain testable.
    if (widget.postLinks != null) _postLinks.start();
    _lastLogoutVersion = widget.sessions.logoutVersion;
    _postLinks.addListener(_schedulePostLink);
    widget.identities.addListener(_schedulePostLink);
    widget.directory.addListener(_schedulePostLink);
    _schedulePostLink();
    _authPresentation.addListener(_syncNotificationLanguage);
    _syncNotificationLanguage();
    _activeIdentity = ActiveIdentityController(widget.identities);
    widget.posts.bindActiveIdentity(_activeIdentity);
    widget.governance?.bindActiveIdentity(_activeIdentity);
    if (widget.presentation == null) unawaited(_authPresentation.load());
    WidgetsBinding.instance.addObserver(this);
    _postRouteChanges = Listenable.merge([_postWindow, widget.directory]);
    _postRouteChanges.addListener(_scheduleAuthCompletion);
    widget.posts.addListener(_postWindowChanged);
    _postWindowChanged();
    widget.sessions.addListener(_sessionRouteChanged);
    _sessionRouteChanged();
    _startAuthentication();
    widget.dm?.controller.addListener(_scheduleDmNotification);
    _navigationUnread = ValueNotifier(hhNoNavigationUnread);
    widget.governance?.addListener(_syncNavigationUnread);
    widget.dm?.controller.addListener(_syncNavigationUnread);
    _syncNavigationUnread();
    widget.dmNotifications?.onTap = _scheduleDmNotification;
    unawaited(
      widget.dmNotifications?.start().then((_) => _scheduleDmNotification()),
    );
  }

  String? _notificationLanguage;
  void _syncNotificationLanguage() {
    final language = _authPresentation.language.code;
    if (_notificationLanguage == language) return;
    _notificationLanguage = language;
    final notifications = widget.dmNotifications;
    if (notifications != null) unawaited(notifications.setLanguage(language));
  }

  void _postWindowChanged() {
    _schedulePostLink();
    _scheduleAuthCompletion();
    final posts = widget.posts;
    final account = posts.accountId;
    if (posts.available && account != null && posts.timelineLoading) {
      _homeChannels[account] = posts.timelineChannelId;
    }
    // Data/loading notifications can arrive while a different route builds.
    // Only a changed authority window invalidates the route's local state.
    _postWindow.value = (widget.posts.accountId, widget.posts.authorityVersion);
  }

  final _sessionRouteWindow = ValueNotifier((AuthStatus.starting, 0, false));
  bool _logoutRoutePending = false, _logoutRouteScheduled = false;
  bool _suppressAutomaticAuthAfterLogout = false;
  void _sessionRouteChanged() {
    final sessions = widget.sessions;
    if (_lastLogoutVersion != sessions.logoutVersion) {
      _lastLogoutVersion = sessions.logoutVersion;
      _postLinks.clear();
      _promptedPostLink = null;
      _logoutRoutePending = true;
      _suppressAutomaticAuthAfterLogout = true;
    }
    if (sessions.isAuthenticated) _suppressAutomaticAuthAfterLogout = false;
    _schedulePostLink();
    // Expiry persistence still notifies its consumers, but renewing the same
    // session must not rebuild every retained tab behind the active detail.
    _sessionRouteWindow.value = (
      sessions.status,
      sessions.authorityVersion,
      sessions.isAuthenticated,
    );
    _scheduleAuthCompletion();
    if (_logoutRoutePending &&
        !sessions.busy &&
        !sessions.isAuthenticated &&
        !_logoutRouteScheduled) {
      _logoutRouteScheduled = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _logoutRouteScheduled = false;
        if (!mounted ||
            !_logoutRoutePending ||
            widget.sessions.busy ||
            widget.sessions.isAuthenticated) {
          return;
        }
        _logoutRoutePending = false;
        final storageFailure =
            widget.sessions.status == AuthStatus.storageFailure;
        if (storageFailure && _authRouteOpen) return;
        final navigator = _navigatorKey.currentState;
        if (navigator == null) {
          _logoutRoutePending = true;
          return;
        }
        navigator.popUntil((route) => route.isFirst);
        if (storageFailure) _openAuth(allowAfterLogout: true);
      });
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      unawaited(widget.flows.refreshPasskeySupport());
      unawaited(widget.sessions.onActivity());
      unawaited(widget.dm?.controller.sync());
      final governance = widget.governance;
      if (governance != null && governance.available) {
        unawaited(governance.refreshInboxUnread());
      }
    }
  }

  Future<void> _startAuthentication() async {
    await widget.sessions.start();
    await widget.flows.restorePending();
    if (!mounted) return;
    if ({
      AuthStatus.closurePending,
      AuthStatus.resetPending,
      AuthStatus.unavailable,
      AuthStatus.storageFailure,
    }.contains(widget.sessions.status)) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _openAuth();
      });
    }
  }

  void _openAuth({bool allowAfterLogout = false}) {
    final navigator = _navigatorKey.currentState;
    if (navigator == null ||
        _authRouteOpen ||
        _suppressAutomaticAuthAfterLogout && !allowAfterLogout) {
      return;
    }
    _authRouteOpen = true;
    navigator
        .push(
          MaterialPageRoute<void>(
            builder: (context) => AuthScreen(
              captchaApi: widget.api,
              sessions: widget.sessions,
              flows: widget.flows,
              onModerationAccess: widget.governance?.restrictedAccess == null
                  ? null
                  : () => unawaited(_openMessages()),
              onAuthenticated: () {
                _authCompletionPending = true;
                _scheduleAuthCompletion();
              },
              onManageSecurity: _openSecurity,
            ),
          ),
        )
        .whenComplete(() {
          _authCompletionPending = false;
          _authRouteOpen = false;
          if (mounted) _schedulePostLink();
        });
  }

  void _openSecurity() {
    final navigator = _navigatorKey.currentState;
    if (navigator == null || _securityRouteOpen) return;
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    _securityRouteOpen = true;
    navigator
        .push(
          HhSlidePageRoute<void>(
            builder: (context) => SecurityManagementScreen(
              controller: widget.management,
              sessions: widget.sessions,
              onAuthenticationRequired: () {
                Navigator.of(context).pop();
                _openAuth();
              },
            ),
          ),
        )
        .whenComplete(() => _securityRouteOpen = false);
  }

  void _openDevices() {
    final navigator = _navigatorKey.currentState;
    if (navigator == null || _devicesRouteOpen) return;
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    _devicesRouteOpen = true;
    final controller = DeviceManagementController(
      api: widget.api,
      store: widget.store,
      sessions: widget.sessions,
    );
    final route = HhSlidePageRoute<void>(
      builder: (context) => DeviceManagementScreen(
        controller: controller,
        sessions: widget.sessions,
        onAuthenticationRequired: () {
          Navigator.of(context).pop();
          _openAuth();
        },
      ),
    );
    navigator.push(route).whenComplete(() async {
      await route.completed;
      controller.dispose();
      _devicesRouteOpen = false;
    });
  }

  void _openClosure() {
    final navigator = _navigatorKey.currentState;
    if (navigator == null || _closureRouteOpen) return;
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    _closureRouteOpen = true;
    navigator
        .push(
          HhSlidePageRoute<void>(
            builder: (context) => AccountClosureScreen(
              sessions: widget.sessions,
              flows: widget.flows,
              onAuthenticationRequired: () {
                Navigator.of(context).pop();
                _openAuth();
              },
            ),
          ),
        )
        .whenComplete(() => _closureRouteOpen = false);
  }

  Widget _settingsScreen(BuildContext context, int authority) {
    return SettingsScreen(
      sessions: widget.sessions,
      sessionAuthority: authority,
      onManageIdentity: _openIdentity,
      onManageSecurity: _openDevices,
      onLogout: () => unawaited(_logoutFromSettings(context)),
      onCloseAccount: _openClosure,
      onChangePassword: _openPasswordReset,
      accountId: widget.store.current?.session?.accountId,
      username: widget.sessions.username,
      onBlacklist: !widget.posts.supportsAuthorBlock
          ? null
          : () => Navigator.of(context).push(
              HhSlidePageRoute<void>(
                builder: (_) => DmBlacklistScreen(posts: widget.posts),
              ),
            ),
    );
  }

  Future<void> _openSettings() async {
    final navigator = _navigatorKey.currentState;
    if (navigator == null || _settingsRouteOpen) return;
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    final authority = widget.sessions.authorityVersion;
    _settingsRouteOpen = true;
    // 设置页从左向右滑入并覆盖当前页，返回时向左滑出。
    final popped = navigator.push<void>(
      PageRouteBuilder<void>(
        settings: const RouteSettings(name: 'settings'),
        transitionDuration: HhMotion.pageSlide,
        reverseTransitionDuration: HhMotion.pageSlide,
        pageBuilder: (context, animation, secondaryAnimation) =>
            _settingsScreen(context, authority),
        transitionsBuilder: (context, animation, secondaryAnimation, child) {
          final curved = CurvedAnimation(
            parent: animation,
            curve: HhMotion.pageSlideCurve,
            reverseCurve: HhMotion.pageSlideCurve,
          );
          return SlideTransition(
            position: curved.drive(
              Tween(begin: const Offset(-1, 0), end: Offset.zero),
            ),
            child: child,
          );
        },
      ),
    );
    await popped.whenComplete(() => _settingsRouteOpen = false);
  }

  void _openPasswordReset() {
    final navigator = _navigatorKey.currentState;
    final token = widget.store.current?.session?.token;
    if (navigator == null ||
        _authRouteOpen ||
        token == null ||
        !widget.sessions.isCurrentToken(token)) {
      return;
    }
    _authRouteOpen = true;
    navigator
        .push(
          HhSlidePageRoute<void>(
            builder: (context) => AuthScreen(
              captchaApi: widget.api,
              sessions: widget.sessions,
              flows: widget.flows,
              passwordChangeToken: token,
              onAuthenticated: () {
                _authCompletionPending = true;
                _scheduleAuthCompletion();
              },
            ),
          ),
        )
        .whenComplete(() {
          _authCompletionPending = false;
          _authRouteOpen = false;
          if (mounted) _schedulePostLink();
        });
  }

  Future<void> _logoutFromSettings(BuildContext context) async {
    if (widget.sessions.busy) return;
    Navigator.of(context).popUntil((route) => route.isFirst);
    await widget.sessions.logout();
  }

  String _identitySnapshot() =>
      widget.identities.directory?.identities
          .map(
            (identity) =>
                '${identity.id}:${identity.nickname}:${identity.avatar}',
          )
          .join('|') ??
      '';

  ManagedIdentity? _findIdentity(String id) {
    for (final identity
        in widget.identities.directory?.identities ??
            const <ManagedIdentity>[]) {
      if (identity.id == id) return identity;
    }
    return null;
  }

  Future<void> _refreshPersonalAfterIdentityChange(
    String before,
    String? account,
    int authority,
  ) async {
    if (widget.posts.accountId == account &&
        widget.posts.authorityVersion == authority &&
        before != _identitySnapshot()) {
      // 身份管理返回后读取当前投影；选择／创建身份仍不会自动发帖。
      await widget.posts.refreshPersonalProjections();
      await widget.posts.refreshKnownPosts();
    }
  }

  Future<void> _setPersonalAvatar() async {
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    final controller = widget.identities;
    if (widget.sessions.busy || controller.busy) return;
    final personal = widget.posts.personalIdentity;
    if (personal == null) return;
    var identity = _findIdentity(personal.id);
    if (identity == null) {
      await controller.load();
      identity = _findIdentity(personal.id);
    }
    if (identity == null) return;
    final account = widget.posts.accountId;
    final authority = widget.posts.authorityVersion;
    final before = _identitySnapshot();
    // 内嵌相册选图 + 圆形裁切编辑；取消时无返回值。
    final context0 = _navigatorKey.currentContext;
    if (context0 == null || !context0.mounted) return;
    Uint8List? prepared;
    try {
      prepared = await pickIdentityAvatarImage(context0);
    } on Object {
      // 相册通道或裁切页异常（含未随完整重新构建注册的插件）时给出反馈，
      // 避免点击后静默无反应。
      _notifyAvatarFailure('无法完成头像选择，请重试。');
      return;
    }
    if (prepared == null) return;
    try {
      final id = newAvatarAssetId();
      if (widget.posts.accountId != account ||
          widget.posts.authorityVersion != authority ||
          !widget.sessions.isAuthenticated) {
        return;
      }
      final avatar = await controller.uploadAvatar(
        id,
        prepared,
        'image/png',
        target: identity.id,
      );
      if (avatar == null) {
        _notifyAvatarFailure(controller.errorMessage ?? '头像上传结果尚未确认，请重试。');
        return;
      }
      await controller.changeAvatar(identity.id, avatar);
      if (widget.identities.hasPending) {
        _notifyAvatarFailure('头像更换结果尚未确认，稍后可在身份管理中核对。');
      }
    } on Object {
      // 裁切输出恒为 ≤640px 的 PNG，不会超限；此处失败是上传/网络问题，
      // 展示服务端错误或统一的上传失败提示。
      _notifyAvatarFailure(controller.errorMessage ?? '头像上传失败，请稍后重试。');
    }
    await _refreshPersonalAfterIdentityChange(before, account, authority);
  }

  void _notifyAvatarFailure(String message) {
    final context = _navigatorKey.currentContext;
    if (context == null || !context.mounted) return;
    // 复用头像流程统一的 3 秒自动消失 + 手动关闭提示。
    notifyAvatarBlocked(context, message);
  }

  Future<void> _editPersonalIdentity() async {
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    final controller = widget.identities;
    if (widget.sessions.busy || controller.busy) return;
    final personal = widget.posts.personalIdentity;
    if (personal == null) return;
    var identity = _findIdentity(personal.id);
    if (identity == null) {
      await controller.load();
      identity = _findIdentity(personal.id);
    }
    if (identity == null) return;
    final context = _navigatorKey.currentContext;
    if (context == null || !context.mounted) return;
    final account = widget.posts.accountId;
    final authority = widget.posts.authorityVersion;
    final before = _identitySnapshot();
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      builder: (context) => IdentityEditor(
        controller: controller,
        sessions: widget.sessions,
        identity: identity,
        avatarEditable: false,
      ),
    );
    await _refreshPersonalAfterIdentityChange(before, account, authority);
  }

  Future<void> _openIdentity({bool initialCreate = false}) async {
    final navigator = _navigatorKey.currentState;
    if (navigator == null || _identityRouteOpen) return;
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    _identityRouteOpen = true;
    final account = widget.posts.accountId;
    final authority = widget.posts.authorityVersion;
    final before = _identitySnapshot();
    await navigator
        .push(
          HhSlidePageRoute<void>(
            builder: (context) => IdentityManagementScreen(
              controller: widget.identities,
              sessions: widget.sessions,
              initialCreate: initialCreate,
              onAuthenticationRequired: () {
                Navigator.of(context).pop();
                _openAuth();
              },
            ),
          ),
        )
        .whenComplete(() => _identityRouteOpen = false);
    await _refreshPersonalAfterIdentityChange(before, account, authority);
  }

  void _openPersonal() {
    if (!widget.sessions.isAuthenticated) {
      _openAuth();
      return;
    }
    _primaryTabs.currentState?.select(AppDestination.personal);
  }

  Widget _personalScreen() {
    final window = _postWindow.value;
    final account = window.$1, authority = window.$2;
    return PersonalPostsScreen(
      showNavigation: false,
      key: ValueKey(window),
      controller: widget.posts,
      loadImages: widget.loadPostImages,
      channelNames: {
        for (final channel in widget.directory.channels)
          channel.id: channel.displayName,
      },
      onManageIdentities: _openIdentity,
      onAuthenticationRequired: _openAuth,
      onOpenSettings: _openSettings,
      onEditIdentity: _editPersonalIdentity,
      onSetAvatar: _setPersonalAvatar,
      onChooseChannel: () =>
          _primaryTabs.currentState?.select(AppDestination.home),
      onPublishedToChannel: (channelId) {
        if (!widget.posts.available ||
            widget.posts.accountId != account ||
            widget.posts.authorityVersion != authority) {
          return;
        }
        if (!widget.directory.channels.any(
          (channel) => channel.id == channelId,
        )) {
          return;
        }
        unawaited(widget.posts.loadTimeline(channelId: channelId));
        _navigatorKey.currentState?.popUntil((route) => route.isFirst);
        _primaryTabs.currentState?.select(AppDestination.home);
      },
    );
  }

  @override
  void dispose() {
    _postLinks.removeListener(_schedulePostLink);
    widget.identities.removeListener(_schedulePostLink);
    widget.directory.removeListener(_schedulePostLink);
    _postLinks.dispose();
    widget.posts.bindActiveIdentity(null);
    widget.governance?.bindActiveIdentity(null);
    _activeIdentity.dispose();
    widget.dm?.controller.removeListener(_scheduleDmNotification);
    widget.governance?.removeListener(_syncNavigationUnread);
    widget.dm?.controller.removeListener(_syncNavigationUnread);
    _navigationUnread.dispose();
    widget.dmNotifications?.dispose();
    widget.dm?.dispose();
    _authPresentation.removeListener(_syncNotificationLanguage);
    _authPresentation.dispose();
    WidgetsBinding.instance.removeObserver(this);
    widget.comments?.dispose();
    widget.posts.removeListener(_postWindowChanged);
    _postRouteChanges.removeListener(_scheduleAuthCompletion);
    _postWindow.dispose();
    widget.management.dispose();
    widget.identities.dispose();
    widget.flows.dispose();
    widget.postMedia?.dispose();
    widget.governance?.dispose();
    widget.governance?.restrictedAccess?.dispose();
    widget.governanceApi?.close();
    widget.posts.dispose();
    widget.postApi.close();
    widget.sessions.removeListener(_sessionRouteChanged);
    _sessionRouteWindow.dispose();
    widget.sessions.dispose();
    widget.api.close();
    widget.directory.dispose();
    _avatars.dispose();
    widget.repository.close();
    widget.store.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _authPresentation,
      builder: (context, _) => MaterialApp(
        initialRoute: '/',
        title: 'SpeechFUL',
        debugShowCheckedModeBanner: false,
        locale: _authPresentation.language.locale,
        supportedLocales: appSupportedLocales,
        localizationsDelegates: appLocalizationDelegates,
        navigatorKey: _navigatorKey,
        themeMode: _authPresentation.themeMode,
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(context).copyWith(
            textScaler: _AppTextScaler(
              MediaQuery.textScalerOf(context),
              _authPresentation.fontScaleValue,
            ),
          ),
          child: PostSharingScope(
            send: widget.sharePostText ?? PostSharingScope.nativeShare,
            child: IdentityAvatarScope(
              repository: _avatars,
              child: DmScope(
                runtime: widget.dm,
                child: AuthPresentationScope(
                  controller: _authPresentation,
                  child: CommentScopeHost(
                    runtime: widget.comments,
                    child: Listener(
                      behavior: HitTestBehavior.translucent,
                      onPointerUp: (_) {
                        // Let the requested action (including logout) start first. Activity
                        // is a user gesture, never a background timer or a build side effect.
                        scheduleMicrotask(widget.sessions.onActivity);
                      },
                      child: CommunitySpokenLocale(
                        child: widget.governance == null
                            ? child ?? const SizedBox.shrink()
                            : GovernanceScope(
                                controller: widget.governance!,
                                child: child ?? const SizedBox.shrink(),
                              ),
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
        theme: HhTelegramTheme.applyAppearance(ThemeData(useMaterial3: true)),
        darkTheme: HhTelegramTheme.applyAppearance(
          ThemeData(useMaterial3: true, brightness: Brightness.dark),
        ),
        home: AnimatedBuilder(
          animation: Listenable.merge([
            _sessionRouteWindow,
            _postRouteChanges,
            if (widget.governance != null) widget.governance!,
          ]),
          builder: (context, _) {
            Widget home;
            if (widget.sessions.isAuthenticated &&
                widget.posts.available &&
                widget.directory.status == ChannelDirectoryStatus.ready) {
              home = IdentityInitialGate(
                controller: widget.identities,
                sessions: widget.sessions,
                child: PrimaryTabs(
                  key: _primaryTabs,
                  window: _postWindow.value,
                  onSelected: _selectPrimary,
                  unread: _navigationUnread,
                  builders: {
                    AppDestination.home: (_) => FeedScreen(
                      // New authorization owns fresh protected data. Its channel
                      // preference survives within this app run for the same account.
                      key: ValueKey(_postWindow.value),
                      showNavigation: false,
                      controller: widget.posts,
                      channels: widget.directory.channels,
                      initialChannelId: _homeChannels[widget.posts.accountId],
                      loadImages: widget.loadPostImages,
                      onOpenMessages: widget.governance == null
                          ? null
                          : _openMessages,
                      onOpenPersonal: _openPersonal,
                      onOpenDirectMessages: _openDirectMessages,
                      onManageIdentities: _openIdentity,
                      onAuthenticationRequired: _openAuth,
                    ),
                    AppDestination.messages: (_) => widget.governance == null
                        ? Center(child: Text(hhText(context, '系统通知暂时不可用')))
                        : _notificationsScreen(),
                    AppDestination.directMessages: (_) => widget.dm != null
                        ? DmInboxRoute(
                            key: ValueKey((
                              _postWindow.value,
                              _dmInboxRevision,
                            )),
                            runtime: widget.dm!,
                            activeIdentity: _activeIdentity,
                            initialIdentity: _dmInitialIdentity,
                            onManageIdentities: () =>
                                _openIdentity(initialCreate: true),
                          )
                        : const DirectMessagesUnavailableScreen(),
                    AppDestination.personal: (_) => _personalScreen(),
                  },
                ),
              );
            } else {
              home = EntryScreen(
                directory: widget.directory,
                showFailureDialog:
                    !_logoutRoutePending &&
                    widget.sessions.status != AuthStatus.signedOut,
                restoringSession:
                    widget.sessions.status == AuthStatus.starting ||
                    widget.sessions.status == AuthStatus.restoring,
                onLoginRequested: () => _openAuth(allowAfterLogout: true),
                onMessagesPressed: widget.governance == null
                    ? null
                    : _openMessages,
                messagesUnread: widget.governance?.hasUnread == true,
              );
            }
            return widget.sessions.status == AuthStatus.signedOut
                ? StudentLetterGate(store: _studentLetters, child: home)
                : home;
          },
        ),
      ),
    );
  }
}

class CommentScopeHost extends StatelessWidget {
  const CommentScopeHost({super.key, this.runtime, required this.child});
  final CommentRuntime? runtime;
  final Widget child;
  @override
  Widget build(BuildContext context) =>
      runtime == null ? child : CommentScope(runtime: runtime!, child: child);
}
